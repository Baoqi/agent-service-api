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
	// Agent name for multi-tenant isolation (required)
	AgentName string

	// User prompt to send to AI (required)
	Prompt string

	// Unique identifier for this execution (required)
	SenderID string

	// Working directory for the AI process (required)
	WorkingDir string

	// Task ID to resume a previous conversation (optional)
	// When provided, the service will look up the session state from the parent task
	ResumeTaskID string

	// System prompt / context to prepend (optional)
	SystemPrompt string

	// Execution configuration (optional)
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

	// Process exit code
	ExitCode int

	// Execution duration
	Duration time.Duration

	// Status: "completed", "failed", "cancelled", "timeout"
	Status string

	// Error message if failed
	Error string

	// Whether this task can be resumed
	IsResumable bool
}

// StreamMessage represents a streaming message
type StreamMessage struct {
	// Message type: "progress", "error", "system", "result", "task_started"
	Type string

	// Message content
	Content string

	// Task ID (available after task_started message)
	TaskID string

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
	AgentName     string
	ParentTaskID  string // Parent task ID if this is a resumed task
	IsResumable   bool   // Whether this task can be resumed
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
	// Validate required fields
	if req.AgentName == "" {
		return fmt.Errorf("agent_name is required")
	}

	// Convert to protobuf request
	pbReq := &pb.ExecuteRequest{
		AgentName:    req.AgentName,
		Prompt:       req.Prompt,
		SenderId:     req.SenderID,
		WorkingDir:   req.WorkingDir,
		ResumeTaskId: req.ResumeTaskID,
		SystemPrompt: req.SystemPrompt,
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
			TaskID:  resp.TaskId,
		}

		switch resp.Type {
		case pb.MessageType_MESSAGE_TYPE_PROGRESS:
			msg.Type = "progress"
		case pb.MessageType_MESSAGE_TYPE_ERROR:
			msg.Type = "error"
		case pb.MessageType_MESSAGE_TYPE_SYSTEM:
			msg.Type = "system"
		case pb.MessageType_MESSAGE_TYPE_TASK_STARTED:
			msg.Type = "task_started"
		case pb.MessageType_MESSAGE_TYPE_RESULT:
			msg.Type = "result"
			if resp.Result != nil {
				msg.Result = &ExecuteResult{
					Output:      resp.Result.Output,
					ExitCode:    int(resp.Result.ExitCode),
					Duration:    time.Duration(resp.Result.DurationSeconds * float64(time.Second)),
					Status:      resp.Result.Status,
					Error:       resp.Result.Error,
					IsResumable: resp.Result.IsResumable,
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

// ListTasks returns all active tasks for the specified agent
func (c *Client) ListTasks(ctx context.Context, agentName string) ([]*TaskInfo, error) {
	if agentName == "" {
		return nil, fmt.Errorf("agent_name is required")
	}

	resp, err := c.client.ListTasks(ctx, &pb.ListTasksRequest{
		AgentName: agentName,
	})
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
			AgentName:     t.AgentName,
			ParentTaskID:  t.ParentTaskId,
			IsResumable:   t.IsResumable,
		})
	}

	return tasks, nil
}

// CancelBySender cancels a running task by sender ID (recommended)
func (c *Client) CancelBySender(ctx context.Context, agentName, senderID string) error {
	if agentName == "" {
		return fmt.Errorf("agent_name is required")
	}

	resp, err := c.client.CancelBySender(ctx, &pb.CancelBySenderRequest{
		AgentName: agentName,
		SenderId:  senderID,
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
func (c *Client) CancelTask(ctx context.Context, agentName, taskID string) error {
	if agentName == "" {
		return fmt.Errorf("agent_name is required")
	}

	resp, err := c.client.CancelTask(ctx, &pb.CancelTaskRequest{
		AgentName: agentName,
		TaskId:    taskID,
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
