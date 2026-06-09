package repository

import (
	"strings"
	"testing"
)

func TestInsertOrchTaskWithReplySQLCastsReplyAnchors(t *testing.T) {
	if !strings.Contains(insertOrchTaskWithReplySQL, "NULLIF($11, '')::uuid") {
		t.Fatalf("source_message_id insert must cast optional reply anchor to uuid")
	}
	if !strings.Contains(insertOrchTaskWithReplySQL, "NULLIF($12, '')::uuid") {
		t.Fatalf("dispatch_message_id insert must cast optional reply anchor to uuid")
	}
}
