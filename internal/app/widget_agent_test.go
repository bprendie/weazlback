package app

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
)

func TestAgentStatusDoesNotRequireRepositoryOperation(t *testing.T) {
	server, client := net.Pipe()
	done := make(chan struct{})
	operation := &sync.Mutex{}
	operation.Lock()

	go func() {
		defer close(done)
		serveAgentRequest(context.Background(), server, operation, config.Config{}, nil, nil)
	}()

	if err := json.NewEncoder(client).Encode(agentRequest{Action: "status"}); err != nil {
		t.Fatal(err)
	}
	var response agentResponse
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Error != "" {
		t.Fatalf("response=%+v", response)
	}
	client.Close()
	operation.Unlock()
	<-done
}
