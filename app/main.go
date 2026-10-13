package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	var prompt string
	flag.StringVar(&prompt, "p", "", "Prompt to send to LLM")
	flag.Parse()

	if prompt == "" {
		panic("Prompt must not be empty")
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	baseUrl := os.Getenv("OPENROUTER_BASE_URL")
	if baseUrl == "" {
		baseUrl = "https://openrouter.ai/api/v1"
	}

	if apiKey == "" {
		panic("Env variable OPENROUTER_API_KEY not found")
	}

	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Fprintln(os.Stderr, "Logs from your program will appear here!")

	skillsMessage, msgErr := skillInstructions()
	if msgErr != nil {
		fmt.Fprintln(os.Stderr, msgErr)
		os.Exit(1)
	}

	client := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseUrl))
	var messages []openai.ChatCompletionMessageParamUnion
	if skillsMessage != "" {
		messages = append(messages, openai.ChatCompletionMessageParamUnion{
			OfSystem: &openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: openai.String(skillsMessage),
				},
			},
		})
	}

	messages = append(messages, openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfString: openai.String(prompt),
			},
		},
	})

	for {
		resp, err := client.Chat.Completions.New(context.Background(),
			openai.ChatCompletionNewParams{
				Model:    "anthropic/claude-haiku-4.5",
				Messages: messages,
				Tools: []openai.ChatCompletionToolUnionParam{
					openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
						Name:        "Read",
						Description: openai.String("Read and return the contents of a file"),
						Parameters: openai.FunctionParameters{
							"type": "object",
							"properties": map[string]any{
								"file_path": map[string]string{
									"type":        "string",
									"description": "The path to the file to read",
								},
							},
							"required": []string{"file_path"},
						},
					}),
					openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
						Name:        "Write",
						Description: openai.String("Write content to a file"),
						Parameters: openai.FunctionParameters{
							"type": "object",
							"properties": map[string]any{
								"file_path": map[string]string{
									"type":        "string",
									"description": "The path to the file to write",
								},
								"content": map[string]string{
									"type":        "string",
									"description": "The content to write to the file",
								},
							},
							"required": []string{"file_path", "content"},
						},
					}),
					openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
						Name:        "Bash",
						Description: openai.String("Execute a shell command"),
						Parameters: openai.FunctionParameters{
							"type": "object",
							"properties": map[string]any{
								"command": map[string]string{
									"type":        "string",
									"description": "The command to execute",
								},
							},
							"required": []string{"command"},
						},
					}),
				},
			},
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Choices) == 0 {
			panic("No choices in response")
		}

		for _, choice := range resp.Choices {
			message := choice.Message

			switch choice.FinishReason {
			case "tool_calls":
				toolCallsParam := make([]openai.ChatCompletionMessageToolCallUnionParam, len(message.ToolCalls))
				for i, tool := range message.ToolCalls {
					toolCallsParam[i] = tool.ToParam()
				}
				messages = append(
					messages,
					openai.ChatCompletionMessageParamUnion{
						OfAssistant: &openai.ChatCompletionAssistantMessageParam{
							ToolCalls: toolCallsParam,
							Role:      "assistant",
						},
					},
				)

				for _, tool := range message.ToolCalls {
					if tool.Function.Name == "Read" {
						var toolArgs struct {
							FilePath string `json:"file_path"`
						}

						parseErr := json.Unmarshal([]byte(tool.Function.Arguments), &toolArgs)
						if parseErr != nil {
							panic(parseErr)
						}

						content, readErr := os.ReadFile(toolArgs.FilePath)
						if readErr != nil {
							panic(readErr)
						}

						messages = append(messages, openai.ChatCompletionMessageParamUnion{
							OfTool: &openai.ChatCompletionToolMessageParam{
								Content: openai.ChatCompletionToolMessageParamContentUnion{
									OfString: openai.String(string(content)),
								},
								ToolCallID: tool.ID,
								Role:       "tool",
							},
						})
					}

					if tool.Function.Name == "Write" {
						var toolArgs struct {
							FilePath string `json:"file_path"`
							Content  string `json:"content"`
						}

						parseErr := json.Unmarshal([]byte(tool.Function.Arguments), &toolArgs)
						if parseErr != nil {
							panic(parseErr)
						}

						writeErr := os.WriteFile(toolArgs.FilePath, []byte(toolArgs.Content), 0644)
						if writeErr != nil {
							panic(writeErr)
						}

						messages = append(messages, openai.ChatCompletionMessageParamUnion{
							OfTool: &openai.ChatCompletionToolMessageParam{
								Content: openai.ChatCompletionToolMessageParamContentUnion{
									OfString: openai.String("Created the file"),
								},
								ToolCallID: tool.ID,
								Role:       "tool",
							},
						})
					}

					if tool.Function.Name == "Bash" {
						var toolArgs struct {
							Command string `json:"command"`
						}

						parseErr := json.Unmarshal([]byte(tool.Function.Arguments), &toolArgs)
						if parseErr != nil {
							panic(parseErr)
						}

						cmd := exec.Command("bash", "-c", toolArgs.Command)
						execOut, execErr := cmd.CombinedOutput()
						if execErr != nil {
							messages = append(messages, openai.ChatCompletionMessageParamUnion{
								OfTool: &openai.ChatCompletionToolMessageParam{
									Content: openai.ChatCompletionToolMessageParamContentUnion{
										OfString: openai.String(execErr.Error()),
									},
									ToolCallID: tool.ID,
									Role:       "tool",
								},
							})
						} else {
							messages = append(messages, openai.ChatCompletionMessageParamUnion{
								OfTool: &openai.ChatCompletionToolMessageParam{
									Content: openai.ChatCompletionToolMessageParamContentUnion{
										OfString: openai.String(string(execOut)),
									},
									ToolCallID: tool.ID,
									Role:       "tool",
								},
							})
						}
					}
				}
			case "stop":
				fmt.Print(message.Content)
				os.Exit(0)
			default:
				fmt.Print(message.Content)
			}
		}
	}
}
