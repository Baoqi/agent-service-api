package client

import (
	"context"
	"fmt"
	"io"
	"time"

	pb "github.com/Baoqi/agent-service-api/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is the Agent Service gRPC client
type Client struct {
	conn   *grpc.ClientConn
	client pb.AgentServiceClient
}

// ExecuteRequest represents a request to execute an AI task
type ExecuteRequest struct {
	// User prompt to send to AI (required)
	Prompt string

	// Unique identifier for this execution (required)
	SenderID string

	// Working directory for the AI process (required)
	WorkingDir string

	// AI engine to use: "claude" or "gemini" (default: "claude")
	AIEngine string

	// Session ID to resume a previous conversation
	SessionID string

	// System prompt / context to prepend
	SystemPrompt string

	// Additional environment variables
	ExtraEnv map[string]string

	// Execution configuration
	Config *ExecuteConfig
}

// ExecuteConfig represents execution configuration
type ExecuteConfig struct {
	// Timeout in minutes
	TimeoutMinutes int

	// Maximum conversation turns
	MaxTurns int

	// Allowed tools
	AllowedTools []string

	// Disallowed tools
	DisallowedTools []string

	// MCP config file path
	McpConfig string
}

// ExecuteResult represents the result of an AI execution
type ExecuteResult struct {
	// Full output from AI
	Output string

	// Session ID for resume
	SessionID string

	// Process exit code
	ExitCode int

	// Execution duration
	Duration time.Duration

	// Status: "completed", "failed", "cancelled", "timeout"
	Status string

	// Error message if failed
	Error string
}

// StreamMessage represents a streaming message
type StreamMessage struct {
	// Message type: "progress", "error", "system", "result"
	Type string

	// Message content
	Content string

	// Final result (only set when Type is "result")
	Result *ExecuteResult
}

// MessageHandler is called for each streaming message
type MessageHandler func(msg *StreamMessage)

// TaskInfo represents information about an AI execution task
type TaskInfo struct {
	TaskID        string
	SenderID      string
	AIEngine      string
	WorkingDir    string
	PromptPreview string
	Status        string
	CreatedAt     time.Time
	StartedAt     time.Time
}

// NewClient creates a new Agent Service client
func NewClient(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent service: %w", err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewAgentServiceClient(conn),
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// Execute executes an AI task and returns the final result
// This is a blocking call that waits for completion
func (c *Client) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error) {
	var result *ExecuteResult

	err := c.ExecuteWithHandler(ctx, req, func(msg *StreamMessage) {
		if msg.Type == "result" && msg.Result != nil {
			result = msg.Result
		}
	})

	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, fmt.Errorf("no result received")
	}

	return result, nil
}

// ExecuteWithHandler executes an AI task with a message handler for streaming
func (c *Client) ExecuteWithHandler(ctx context.Context, req *ExecuteRequest, handler MessageHandler) error {
	// Convert to protobuf request
	pbReq := &pb.ExecuteRequest{
		Prompt:       req.Prompt,
		SenderId:     req.SenderID,
		WorkingDir:   req.WorkingDir,
		AiEngine:     req.AIEngine,
		SessionId:    req.SessionID,
		SystemPrompt: req.SystemPrompt,
		ExtraEnv:     req.ExtraEnv,
	}

	if req.Config != nil {
		pbReq.Config = &pb.ExecuteConfig{
			TimeoutMinutes:  int32(req.Config.TimeoutMinutes),
			MaxTurns:        int32(req.Config.MaxTurns),
			AllowedTools:    req.Config.AllowedTools,
			DisallowedTools: req.Config.DisallowedTools,
			McpConfig:       req.Config.McpConfig,
		}
	}

	// Call Execute RPC
	stream, err := c.client.Execute(ctx, pbReq)
	if err != nil {
		return fmt.Errorf("failed to call Execute: %w", err)
	}

	// Receive streaming messages
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error receiving stream: %w", err)
		}

		// Convert to StreamMessage
		msg := &StreamMessage{
			Content: resp.Content,
		}

		switch resp.Type {
		case pb.MessageType_MESSAGE_TYPE_PROGRESS:
			msg.Type = "progress"
		case pb.MessageType_MESSAGE_TYPE_ERROR:
			msg.Type = "error"
		case pb.MessageType_MESSAGE_TYPE_SYSTEM:
			msg.Type = "system"
		case pb.MessageType_MESSAGE_TYPE_RESULT:
			msg.Type = "result"
			if resp.Result != nil {
				msg.Result = &ExecuteResult{
					Output:    resp.Result.Output,
					SessionID: resp.Result.SessionId,
					ExitCode:  int(resp.Result.ExitCode),
					Duration:  time.Duration(resp.Result.DurationSeconds * float64(time.Second)),
					Status:    resp.Result.Status,
					Error:     resp.Result.Error,
				}
			}
		}

		// Call handler
		if handler != nil {
			handler(msg)
		}
	}

	return nil
}

// ListTasks returns all active tasks
func (c *Client) ListTasks(ctx context.Context) ([]*TaskInfo, error) {
	resp, err := c.client.ListTasks(ctx, &pb.ListTasksRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	tasks := make([]*TaskInfo, 0, len(resp.Tasks))
	for _, t := range resp.Tasks {
		tasks = append(tasks, &TaskInfo{
			TaskID:        t.TaskId,
			SenderID:      t.SenderId,
			AIEngine:      t.AiEngine,
			WorkingDir:    t.WorkingDir,
			PromptPreview: t.PromptPreview,
			Status:        t.Status,
			CreatedAt:     time.Unix(t.CreatedAt, 0),
			StartedAt:     time.Unix(t.StartedAt, 0),
		})
	}

	return tasks, nil
}

// CancelBySender cancels a running task by sender ID (recommended)
func (c *Client) CancelBySender(ctx context.Context, senderID string) error {
	resp, err := c.client.CancelBySender(ctx, &pb.CancelBySenderRequest{
		SenderId: senderID,
	})
	if err != nil {
		return fmt.Errorf("failed to cancel by sender: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("cancel failed: %s", resp.Message)
	}

	return nil
}

// CancelTask cancels a running task by task ID
func (c *Client) CancelTask(ctx context.Context, taskID string) error {
	resp, err := c.client.CancelTask(ctx, &pb.CancelTaskRequest{
		TaskId: taskID,
	})
	if err != nil {
		return fmt.Errorf("failed to cancel task: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("cancel failed: %s", resp.Message)
	}

	return nil
}

// Health returns the health status of the service
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	resp, err := c.client.Health(ctx, &pb.HealthRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get health: %w", err)
	}

	return &HealthStatus{
		Status:          resp.Status,
		Version:         resp.Version,
		ActiveProcesses: int(resp.ActiveProcesses),
		Uptime:          time.Duration(resp.UptimeSeconds) * time.Second,
	}, nil
}

// HealthStatus represents the health status of the service
type HealthStatus struct {
	Status          string
	Version         string
	ActiveProcesses int
	Uptime          time.Duration
}

