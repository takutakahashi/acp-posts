package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/slack-go/slack"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Message is the top-level ACP JSONL envelope.
type Message struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message"`
	SessionID string          `json:"sessionId"`
}

// AssistantMessage is the Anthropic API message body.
type AssistantMessage struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Role       string        `json:"role"`
	Model      string        `json:"model"`
	Content    []ContentItem `json:"content"`
	StopReason string        `json:"stop_reason"`
}

// ContentItem is a single item in an assistant message's content array.
type ContentItem struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

var api *slack.Client

func main() {
	pflag.String("bot-token", "", "Slack bot token")
	pflag.String("channel-id", "", "Slack channel ID")
	pflag.String("thread-ts", "", "Slack thread timestamp")
	pflag.String("file", "", "Path to JSONL file to watch")
	pflag.Parse()

	viper.SetEnvPrefix("SLACK")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalf("failed to bind flags: %v", err)
	}

	slackBotToken := viper.GetString("bot-token")
	channelID := viper.GetString("channel-id")
	threadTS := viper.GetString("thread-ts")
	filePath := viper.GetString("file")

	if slackBotToken != "" {
		if channelID == "" || threadTS == "" {
			log.Fatal("--channel-id and --thread-ts are required when --bot-token is set")
		}
		api = slack.New(slackBotToken)
	}

	if filePath != "" {
		watchFile(filePath, channelID, threadTS)
	} else {
		readStdin(channelID, threadTS)
	}
}

func readStdin(channelID, threadTS string) {
	reader := bufio.NewReader(os.Stdin)
	var buf strings.Builder
	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("read error: %v", err)
		}
		if b == '\n' {
			processBuffer(buf.String(), channelID, threadTS)
			buf.Reset()
		} else {
			buf.WriteByte(b)
		}
	}
	if buf.Len() > 0 {
		processBuffer(buf.String(), channelID, threadTS)
	}
}

func watchFile(filePath, channelID, threadTS string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	var offset int64
	offset = processFileFrom(filePath, offset, channelID, threadTS)

	if err := watcher.Add(filePath); err != nil {
		log.Fatalf("failed to watch file: %v", err)
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write != 0 {
				offset = processFileFrom(filePath, offset, channelID, threadTS)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

func processFileFrom(filePath string, offset int64, channelID, threadTS string) int64 {
	f, err := os.Open(filePath)
	if err != nil {
		log.Printf("failed to open file: %v", err)
		return offset
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		log.Printf("failed to seek: %v", err)
		return offset
	}

	var buf strings.Builder
	reader := bufio.NewReader(f)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("read error: %v", err)
			break
		}
		offset++
		if b == '\n' {
			processBuffer(buf.String(), channelID, threadTS)
			buf.Reset()
		} else {
			buf.WriteByte(b)
		}
	}
	return offset
}

func processBuffer(line, channelID, threadTS string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	var msg Message
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return
	}

	if msg.Type != "assistant" {
		return
	}

	var assistantMsg AssistantMessage
	if err := json.Unmarshal(msg.Message, &assistantMsg); err != nil {
		return
	}

	for _, item := range assistantMsg.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			postToSlack(convertMarkdownToSlack(item.Text), channelID, threadTS)
		}
	}
}

func postToSlack(text, channelID, threadTS string) {
	if api == nil {
		fmt.Println(text)
		return
	}
	_, _, err := api.PostMessage(
		channelID,
		slack.MsgOptionTS(threadTS),
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		log.Printf("failed to post message: %v", err)
	}
}

var (
	reBold      = regexp.MustCompile(`\*\*(.*?)\*\*|__(.*?)__`)
	reItalic    = regexp.MustCompile(`(^|[^*])\*([^*\s][^*]*)\*([^*]|$)`)
	reHeader    = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	reStrike    = regexp.MustCompile(`~~(.*?)~~`)
	reLink      = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reCodeBlock = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")
	reTable     = regexp.MustCompile(`(?m)^\|.+\|$`)
)

func convertMarkdownToSlack(text string) string {
	// Preserve code blocks
	codeBlocks := []string{}
	placeholder := "\x00CODE%d\x00"
	text = reCodeBlock.ReplaceAllStringFunc(text, func(match string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, match)
		return fmt.Sprintf(placeholder, idx)
	})

	// Convert tables to code blocks
	text = reTable.ReplaceAllStringFunc(text, func(match string) string {
		return "```\n" + match + "\n```"
	})

	text = reItalic.ReplaceAllString(text, "${1}_${2}_${3}")
	text = reBold.ReplaceAllString(text, "*$1$2*")
	text = reHeader.ReplaceAllString(text, "*$1*")
	text = reStrike.ReplaceAllString(text, "~$1~")
	text = reLink.ReplaceAllString(text, "<$2|$1>")

	// Restore code blocks
	for i, block := range codeBlocks {
		text = strings.ReplaceAll(text, fmt.Sprintf(placeholder, i), block)
	}

	return text
}
