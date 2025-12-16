# Agent Service API

Shared gRPC API definitions and client SDK for the Agent Service.

## Overview

This package provides:
- Protocol Buffer definitions for the Agent Service gRPC API
- Go client SDK for interacting with the Agent Service

## Installation

```bash
go get github.com/Baoqi/agent-service-api
```

## Usage

### Client SDK

```go
package main

import (
    "context"
    "log"

    client "github.com/Baoqi/agent-service-api/pkg/client"
)

func main() {
    // Create client
    c, err := client.NewClient("localhost:50051")
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // Execute AI task
    result, err := c.Execute(context.Background(), &client.ExecuteRequest{
        Prompt:     "Hello, Claude!",
        SenderID:   "my-sender-id",
        WorkingDir: "/path/to/project",
        AIEngine:   "claude",
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Result: %s", result.Output)
}
```

### Streaming Handler

```go
err := c.ExecuteWithHandler(ctx, req, func(msg *client.StreamMessage) {
    switch msg.Type {
    case "progress":
        log.Printf("Progress: %s", msg.Content)
    case "result":
        log.Printf("Done: %s", msg.Result.Status)
    }
})
```

## Development

### Generate Protobuf Code

```bash
make proto
```

### Install Dependencies

```bash
make deps
```

## API Reference

### Execute

Execute an AI task with streaming response.

### ListTasks

List all active tasks.

### CancelTask

Cancel a running task by task ID.

### CancelBySender

Cancel a running task by sender ID.

### Health

Health check endpoint.

