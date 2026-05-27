// Package protosmoke holds build-time sanity tests for the generated proto
// stubs. It lives outside gen/ (which is gitignored) so the test file itself
// can be committed to git, while still importing the generated package.
//
// If `buf generate` has not been run, `go test ./...` will fail with an
// unresolved import — that's intentional and acts as a forcing function to
// keep developers honest about regenerating stubs.
package protosmoke

import (
	"testing"

	runedv1 "github.com/CryptoLabInc/runed/gen/runed/v1"
)

// TestProtoCompiles confirms the generated stubs expose the expected types
// and basic getters work. This catches proto regressions that somehow build
// but produce wrong shapes.
func TestProtoCompiles(t *testing.T) {
	req := &runedv1.EmbedRequest{Text: "hello"}
	if req.GetText() != "hello" {
		t.Fatal("EmbedRequest.GetText broken")
	}

	resp := &runedv1.EmbedResponse{Vector: []float32{1, 2, 3}}
	if len(resp.GetVector()) != 3 {
		t.Fatal("EmbedResponse.GetVector broken")
	}

	info := &runedv1.InfoResponse{VectorDim: 1024}
	if info.GetVectorDim() != 1024 {
		t.Fatal("InfoResponse.GetVectorDim broken")
	}

	// Confirm the service name — this is what downstream clients import.
	// If this fails, someone renamed the service without updating callers.
	var _ runedv1.RunedServiceClient
	var _ runedv1.RunedServiceServer
}
