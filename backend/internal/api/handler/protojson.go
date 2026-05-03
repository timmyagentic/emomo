// Package handler hosts emomo's Gin HTTP handlers.
//
// Handlers stay RESTful (no Connect / gRPC), but their request and response
// bodies are protobuf messages serialized via protojson with
// UseEnumNumbers=true. The frontend consumes the same shapes via
// protoc-gen-es generated TypeScript code.
package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	protojsonMarshal   = protojson.MarshalOptions{UseEnumNumbers: true, UseProtoNames: true}
	protojsonUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}
)

// readProtoJSON reads the entire request body and unmarshals it into msg using
// protojson. An empty body is allowed and results in msg being left at its
// zero value (proto3 default semantics).
func readProtoJSON(c *gin.Context, msg proto.Message) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if len(body) == 0 {
		return nil
	}
	return protojsonUnmarshal.Unmarshal(body, msg)
}

// writeProtoJSON serializes msg into the response with the provided HTTP
// status code and Content-Type: application/json.
func writeProtoJSON(c *gin.Context, status int, msg proto.Message) {
	body, err := protojsonMarshal.Marshal(msg)
	if err != nil {
		writeError(c, http.StatusInternalServerError, fmt.Errorf("marshal response: %w", err))
		return
	}
	c.Data(status, "application/json", body)
}

// writeError writes a uniform error envelope as `{"error":"..."}` JSON. The
// envelope is *not* a protobuf message — clients should not depend on a
// specific shape beyond the `error` string field, which keeps backwards
// compatibility with the original gin.H{"error": ...} contract.
func writeError(c *gin.Context, status int, err error) {
	if err == nil {
		err = errors.New("unknown error")
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
