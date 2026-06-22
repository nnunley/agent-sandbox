package laneqpb

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestRoundTripDirective tests marshal/unmarshal fidelity of Directive messages.
func TestRoundTripDirective(t *testing.T) {
	now := time.Now().Unix()
	notBefore := now + 3600
	original := &Directive{
		Id:               "dir-123",
		Priority:         Priority_PRIORITY_P0,
		Body:             `{"intent":"test","template":"foo","origin":"orchestrator"}`,
		Status:           Status_STATUS_PENDING,
		Lane:             "default",
		CreatedAtUnix:    now,
		TakenAtUnix:      nil, // Optional; not yet taken
		DoneAtUnix:       nil, // Optional; not yet done
		TakenBy:          "",
		LeaseUntilUnix:   nil, // Optional; no active lease
		RequeueCount:     0,
		ParentId:         "",
		NotBeforeUnix:    &notBefore, // Optional; eligible after this time
		BlockedBy:        []string{"dir-456", "dir-789"},
	}

	// Marshal to bytes
	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Unmarshal back
	restored := &Directive{}
	if err := proto.Unmarshal(data, restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify field fidelity
	if restored.Id != original.Id {
		t.Errorf("ID mismatch: got %q, want %q", restored.Id, original.Id)
	}
	if restored.Priority != original.Priority {
		t.Errorf("Priority mismatch: got %v, want %v", restored.Priority, original.Priority)
	}
	if restored.Body != original.Body {
		t.Errorf("Body mismatch: got %q, want %q", restored.Body, original.Body)
	}
	if restored.Status != original.Status {
		t.Errorf("Status mismatch: got %v, want %v", restored.Status, original.Status)
	}
	if restored.Lane != original.Lane {
		t.Errorf("Lane mismatch: got %q, want %q", restored.Lane, original.Lane)
	}
	if restored.CreatedAtUnix != original.CreatedAtUnix {
		t.Errorf("CreatedAtUnix mismatch: got %d, want %d", restored.CreatedAtUnix, original.CreatedAtUnix)
	}
	if (restored.NotBeforeUnix == nil) != (original.NotBeforeUnix == nil) {
		t.Errorf("NotBeforeUnix presence mismatch")
	}
	if restored.NotBeforeUnix != nil && original.NotBeforeUnix != nil && *restored.NotBeforeUnix != *original.NotBeforeUnix {
		t.Errorf("NotBeforeUnix mismatch: got %d, want %d", *restored.NotBeforeUnix, *original.NotBeforeUnix)
	}
	if len(restored.BlockedBy) != len(original.BlockedBy) {
		t.Errorf("BlockedBy length mismatch: got %d, want %d", len(restored.BlockedBy), len(original.BlockedBy))
	}
	for i, v := range restored.BlockedBy {
		if v != original.BlockedBy[i] {
			t.Errorf("BlockedBy[%d] mismatch: got %q, want %q", i, v, original.BlockedBy[i])
		}
	}
}

// TestRoundTripPushRequest tests PushRequest serialization.
func TestRoundTripPushRequest(t *testing.T) {
	original := &PushRequest{
		Body:     `{"intent":"build","repo":"https://example.com/repo"}`,
		Priority: Priority_PRIORITY_P1,
		ParentId: "parent-001",
		Lane:     "builds",
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	restored := &PushRequest{}
	if err := proto.Unmarshal(data, restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if restored.Body != original.Body {
		t.Errorf("Body mismatch: got %q, want %q", restored.Body, original.Body)
	}
	if restored.Priority != original.Priority {
		t.Errorf("Priority mismatch: got %v, want %v", restored.Priority, original.Priority)
	}
	if restored.ParentId != original.ParentId {
		t.Errorf("ParentId mismatch: got %q, want %q", restored.ParentId, original.ParentId)
	}
	if restored.Lane != original.Lane {
		t.Errorf("Lane mismatch: got %q, want %q", restored.Lane, original.Lane)
	}
}

// TestRoundTripTakeRequest tests TakeRequest serialization.
func TestRoundTripTakeRequest(t *testing.T) {
	original := &TakeRequest{
		Consumer:         "worker-42",
		Lane:             "default",
		LeaseDurationMs:  30000,
		ReapStaleSeconds: 3600,
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	restored := &TakeRequest{}
	if err := proto.Unmarshal(data, restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if restored.Consumer != original.Consumer {
		t.Errorf("Consumer mismatch: got %q, want %q", restored.Consumer, original.Consumer)
	}
	if restored.LeaseDurationMs != original.LeaseDurationMs {
		t.Errorf("LeaseDurationMs mismatch: got %d, want %d", restored.LeaseDurationMs, original.LeaseDurationMs)
	}
}

// TestRoundTripDeferRequest tests DeferRequest with dependencies.
func TestRoundTripDeferRequest(t *testing.T) {
	until := int64(1234567890)
	original := &DeferRequest{
		Id:        "dir-123",
		UntilUnix: &until, // Optional; if absent, use delay_ms
		DelayMs:   5000,
		BlockedBy: []string{"dep-1", "dep-2", "dep-3"},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	restored := &DeferRequest{}
	if err := proto.Unmarshal(data, restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if restored.Id != original.Id {
		t.Errorf("Id mismatch: got %q, want %q", restored.Id, original.Id)
	}
	if len(restored.BlockedBy) != len(original.BlockedBy) {
		t.Errorf("BlockedBy length mismatch: got %d, want %d", len(restored.BlockedBy), len(original.BlockedBy))
	}
	for i, v := range restored.BlockedBy {
		if v != original.BlockedBy[i] {
			t.Errorf("BlockedBy[%d] mismatch: got %q, want %q", i, v, original.BlockedBy[i])
		}
	}
}

// TestRoundTripStatsResponse tests StatsResponse with maps.
func TestRoundTripStatsResponse(t *testing.T) {
	original := &StatsResponse{
		ByStatus: map[string]int32{
			"pending":  5,
			"taken":    2,
			"deferred": 1,
			"done":     10,
			"parked":   0,
		},
		Consumers: []*ConsumerStats{
			{Consumer: "worker-1", ActiveLeases: 1, TotalClaimed: 50, TotalCompleted: 48},
			{Consumer: "worker-2", ActiveLeases: 0, TotalClaimed: 45, TotalCompleted: 45},
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	restored := &StatsResponse{}
	if err := proto.Unmarshal(data, restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(restored.ByStatus) != len(original.ByStatus) {
		t.Errorf("ByStatus length mismatch: got %d, want %d", len(restored.ByStatus), len(original.ByStatus))
	}
	if restored.ByStatus["pending"] != original.ByStatus["pending"] {
		t.Errorf("ByStatus['pending'] mismatch: got %d, want %d", restored.ByStatus["pending"], original.ByStatus["pending"])
	}
	if len(restored.Consumers) != len(original.Consumers) {
		t.Errorf("Consumers length mismatch: got %d, want %d", len(restored.Consumers), len(original.Consumers))
	}
}

// TestServiceInterfaceExists verifies the Laneq gRPC service interface is generated.
func TestServiceInterfaceExists(t *testing.T) {
	// Verify the service descriptor exists
	laneqFile := File_laneq_proto
	services := laneqFile.Services()

	if services.Len() == 0 {
		t.Fatal("no services found in laneq.proto")
	}

	// Verify the Laneq service exists
	laneqService := services.ByName("Laneq")
	if laneqService == nil {
		t.Fatal("Laneq service not found")
	}

	// Verify key RPCs exist
	expectedRPCs := map[string]bool{
		"Push":           false,
		"Take":           false,
		"Peek":           false,
		"Show":           false,
		"Listing":        false,
		"Reprioritize":   false,
		"SetStatus":      false,
		"Defer":          false,
		"Touch":          false,
		"Reap":           false,
		"Stats":          false,
		"ThreadStatus":   false,
		"Park":           false,
		"Unpark":         false,
	}

	methods := laneqService.Methods()
	for i := 0; i < methods.Len(); i++ {
		method := methods.Get(i)
		name := string(method.Name())
		if _, ok := expectedRPCs[name]; ok {
			expectedRPCs[name] = true
		}
	}

	for rpc, found := range expectedRPCs {
		if !found {
			t.Errorf("RPC %q not found in Laneq service", rpc)
		}
	}
}

// TestStatusEnumValues verifies Status enum has all required values.
func TestStatusEnumValues(t *testing.T) {
	statusDesc := Status_name
	for _, name := range statusDesc {
		// Just verify the enum names are populated
		if name == "" {
			t.Error("empty status enum name")
		}
	}

	// Verify we can construct Status values without panicking
	statuses := []Status{
		Status_STATUS_UNSPECIFIED,
		Status_STATUS_PENDING,
		Status_STATUS_TAKEN,
		Status_STATUS_DEFERRED,
		Status_STATUS_DONE,
		Status_STATUS_DROPPED,
		Status_STATUS_PARKED,
	}
	if len(statuses) != 7 {
		t.Errorf("expected 7 status values, got %d", len(statuses))
	}
}

// TestPriorityEnumValues verifies Priority enum has all required values.
func TestPriorityEnumValues(t *testing.T) {
	priorities := []Priority{
		Priority_PRIORITY_UNSPECIFIED,
		Priority_PRIORITY_P0,
		Priority_PRIORITY_P1,
		Priority_PRIORITY_P2,
	}
	if len(priorities) != 4 {
		t.Errorf("expected 4 priority values, got %d", len(priorities))
	}
}

// TestMessageDescriptorsExist verifies key message descriptors.
func TestMessageDescriptorsExist(t *testing.T) {
	expectedMessages := []string{
		"Directive",
		"PushRequest",
		"PushResponse",
		"TakeRequest",
		"TakeResponse",
		"PeekRequest",
		"PeekResponse",
		"ShowRequest",
		"ShowResponse",
		"DeferRequest",
		"DeferResponse",
		"TouchRequest",
		"TouchResponse",
		"ParkRequest",
		"ParkResponse",
		"UnparkRequest",
		"UnparkResponse",
		"StatsRequest",
		"StatsResponse",
		"ThreadStatusRequest",
		"ThreadStatusResponse",
	}

	laneqFile := File_laneq_proto
	messages := laneqFile.Messages()

	foundMessages := make(map[string]bool)
	for i := 0; i < messages.Len(); i++ {
		msg := messages.Get(i)
		name := string(msg.Name())
		foundMessages[name] = true
	}

	for _, msgName := range expectedMessages {
		if !foundMessages[msgName] {
			t.Errorf("message %q not found", msgName)
		}
	}
}

// TestDeferRequestHasBlockedByField verifies dependencies are modeled.
func TestDeferRequestHasBlockedByField(t *testing.T) {
	until := int64(1234567890)
	req := &DeferRequest{
		Id:        "test-id",
		UntilUnix: &until, // Optional
		BlockedBy: []string{"dep-1", "dep-2"},
	}

	if len(req.BlockedBy) != 2 {
		t.Errorf("expected 2 blocked_by items, got %d", len(req.BlockedBy))
	}

	if req.BlockedBy[0] != "dep-1" || req.BlockedBy[1] != "dep-2" {
		t.Errorf("blocked_by not set correctly")
	}

	// Verify it survives round-trip
	data, _ := proto.Marshal(req)
	restored := &DeferRequest{}
	proto.Unmarshal(data, restored)

	if len(restored.BlockedBy) != 2 {
		t.Errorf("after unmarshal, expected 2 blocked_by items, got %d", len(restored.BlockedBy))
	}
}

// TestDirectiveHasAllSchedulingColumns verifies scheduling columns are present.
func TestDirectiveHasAllSchedulingColumns(t *testing.T) {
	notBefore := int64(1234567890)
	leaseUntil := int64(1234567999)
	directive := &Directive{
		Id:              "test",
		Priority:        Priority_PRIORITY_P0,
		Status:          Status_STATUS_DEFERRED,
		Lane:            "custom-lane",
		NotBeforeUnix:   &notBefore,  // Optional: set
		BlockedBy:       []string{"dep-1"},
		RequeueCount:    3,
		ParentId:        "parent-id",
		TakenBy:         "consumer-1",
		LeaseUntilUnix:  &leaseUntil,  // Optional: set
	}

	// Verify all fields are accessible
	if directive.Priority != Priority_PRIORITY_P0 {
		t.Error("Priority not set")
	}
	if directive.Lane != "custom-lane" {
		t.Error("Lane not set")
	}
	if directive.NotBeforeUnix == nil {
		t.Error("NotBeforeUnix is nil")
	}
	if len(directive.BlockedBy) == 0 {
		t.Error("BlockedBy is empty")
	}
	if directive.RequeueCount == 0 {
		t.Error("RequeueCount is zero")
	}
	if directive.ParentId == "" {
		t.Error("ParentId is empty")
	}
	if directive.TakenBy == "" {
		t.Error("TakenBy is empty")
	}
	if directive.LeaseUntilUnix == nil {
		t.Error("LeaseUntilUnix is nil")
	}
}

// TestParkedStatusExists verifies the PARKED status is available.
func TestParkedStatusExists(t *testing.T) {
	directive := &Directive{
		Id:     "test",
		Status: Status_STATUS_PARKED,
	}

	if directive.Status != Status_STATUS_PARKED {
		t.Errorf("expected STATUS_PARKED, got %v", directive.Status)
	}

	// Verify it survives round-trip
	data, _ := proto.Marshal(directive)
	restored := &Directive{}
	proto.Unmarshal(data, restored)

	if restored.Status != Status_STATUS_PARKED {
		t.Errorf("after unmarshal, expected STATUS_PARKED, got %v", restored.Status)
	}
}

// TestGrpcServiceMethods verifies the gRPC service has the right method signatures.
func TestGrpcServiceMethods(t *testing.T) {
	// This test verifies that the generated gRPC code has the service interface.
	// The actual interface is generated in laneq_grpc.pb.go as LaneqServer.
	// We verify the descriptor has the right methods.

	laneqFile := File_laneq_proto
	services := laneqFile.Services()

	laneqService := services.ByName("Laneq")
	methods := laneqService.Methods()

	methodMap := make(map[string]protoreflect.MethodDescriptor)
	for i := 0; i < methods.Len(); i++ {
		method := methods.Get(i)
		methodMap[string(method.Name())] = method
	}

	// Spot-check a few methods
	if method, ok := methodMap["Push"]; !ok {
		t.Fatal("Push method not found")
	} else {
		if string(method.Input().Name()) != "PushRequest" {
			t.Errorf("Push input should be PushRequest, got %s", method.Input().Name())
		}
		if string(method.Output().Name()) != "PushResponse" {
			t.Errorf("Push output should be PushResponse, got %s", method.Output().Name())
		}
	}

	if method, ok := methodMap["Park"]; !ok {
		t.Fatal("Park method not found")
	} else {
		if string(method.Input().Name()) != "ParkRequest" {
			t.Errorf("Park input should be ParkRequest, got %s", method.Input().Name())
		}
	}

	if method, ok := methodMap["Unpark"]; !ok {
		t.Fatal("Unpark method not found")
	} else {
		if string(method.Input().Name()) != "UnparkRequest" {
			t.Errorf("Unpark input should be UnparkRequest, got %s", method.Input().Name())
		}
	}
}

// TestProtoFieldNumbersPinned asserts that critical Directive field numbers are stable.
// This catches accidental renumbering that round-trip tests miss.
func TestProtoFieldNumbersPinned(t *testing.T) {
	laneqFile := File_laneq_proto
	directiveMsg := laneqFile.Messages().ByName("Directive")
	if directiveMsg == nil {
		t.Fatal("Directive message not found")
	}

	// Map of field names to their expected field numbers
	expectedFieldNumbers := map[string]int32{
		"id":                1,
		"priority":          2,
		"status":            4,
		"body":              3,
		"lane":              5,
		"not_before_unix":   13,
		"lease_until_unix":  10,
		"taken_by":          9,
		"requeue_count":     11,
		"parent_id":         12,
		"blocked_by":        14,
		"created_at_unix":   6,
		"taken_at_unix":     7,
		"done_at_unix":      8,
	}

	for fieldName, expectedNumber := range expectedFieldNumbers {
		field := directiveMsg.Fields().ByName(protoreflect.Name(fieldName))
		if field == nil {
			t.Errorf("field %q not found in Directive", fieldName)
			continue
		}
		actualNumber := int32(field.Number())
		if actualNumber != expectedNumber {
			t.Errorf("field %q: expected number %d, got %d", fieldName, expectedNumber, actualNumber)
		}
	}
}
