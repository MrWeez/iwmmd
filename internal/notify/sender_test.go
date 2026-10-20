package notify

import (
	"context"
	"testing"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/store"
)

func TestDemoSenderSkipsNetwork(t *testing.T) {
	sender := &Sender{Demo: true}
	status, err := sender.Send(context.Background(), store.Channel{
		Type:             "email",
		ConfigCiphertext: "not-a-valid-ciphertext",
	}, alert.Payload{}, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if status != 202 {
		t.Fatalf("status = %d", status)
	}
}
