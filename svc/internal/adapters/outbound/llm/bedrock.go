package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type BedrockConfig struct {
	Region     string
	Endpoint   string
	ModelID    string
	HTTPClient *http.Client
}

type BedrockClient struct {
	client  *bedrockruntime.Client
	modelID string
}

func NewBedrockClient(ctx context.Context, cfg BedrockConfig) (*BedrockClient, error) {
	if strings.TrimSpace(cfg.ModelID) == "" {
		return nil, fmt.Errorf("bedrock model id is required")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(strings.TrimSpace(cfg.Region)))
	if err != nil {
		return nil, err
	}
	if cfg.HTTPClient != nil {
		awsCfg.HTTPClient = cfg.HTTPClient
	}
	client := bedrockruntime.NewFromConfig(awsCfg, func(o *bedrockruntime.Options) {
		if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	return &BedrockClient{client: client, modelID: strings.TrimSpace(cfg.ModelID)}, nil
}

func (c *BedrockClient) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	return c.ChatCompletionWithOptions(ctx, messages, driven.LLMRequestOptions{})
}

func (c *BedrockClient) ChatCompletionWithOptions(ctx context.Context, messages []driven.LLMMessage, opts driven.LLMRequestOptions) (*driven.LLMResponse, error) {
	if c == nil || c.client == nil || c.modelID == "" {
		return nil, fmt.Errorf("bedrock client not configured")
	}
	systemBlocks := make([]bedrocktypes.SystemContentBlock, 0, 1)
	conversation := make([]bedrocktypes.Message, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		switch role {
		case "system":
			systemBlocks = append(systemBlocks, &bedrocktypes.SystemContentBlockMemberText{Value: content})
		case "assistant":
			conversation = append(conversation, bedrocktypes.Message{
				Role: bedrocktypes.ConversationRoleAssistant,
				Content: []bedrocktypes.ContentBlock{
					&bedrocktypes.ContentBlockMemberText{Value: content},
				},
			})
		default:
			conversation = append(conversation, bedrocktypes.Message{
				Role: bedrocktypes.ConversationRoleUser,
				Content: []bedrocktypes.ContentBlock{
					&bedrocktypes.ContentBlockMemberText{Value: content},
				},
			})
		}
	}
	if len(conversation) == 0 {
		return nil, fmt.Errorf("bedrock requires at least one non-system message")
	}
	maxTokens := int32(1200)
	if opts.MaxOutputTokens > 0 {
		maxTokens = int32(opts.MaxOutputTokens)
	}
	callCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
	}
	out, err := c.client.Converse(callCtx, &bedrockruntime.ConverseInput{
		ModelId:  aws.String(c.modelID),
		System:   systemBlocks,
		Messages: conversation,
		InferenceConfig: &bedrocktypes.InferenceConfiguration{
			Temperature: aws.Float32(0),
			MaxTokens:   aws.Int32(maxTokens),
		},
	})
	if err != nil {
		return nil, err
	}
	msg, ok := out.Output.(*bedrocktypes.ConverseOutputMemberMessage)
	if !ok || msg == nil {
		return nil, fmt.Errorf("bedrock returned no message output")
	}
	var b strings.Builder
	for _, block := range msg.Value.Content {
		text, ok := block.(*bedrocktypes.ContentBlockMemberText)
		if !ok || text == nil {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text.Value)
	}
	if strings.TrimSpace(b.String()) == "" {
		return nil, fmt.Errorf("bedrock returned empty text")
	}
	return &driven.LLMResponse{Content: b.String()}, nil
}

var _ driven.LLMClientWithOptions = (*BedrockClient)(nil)
