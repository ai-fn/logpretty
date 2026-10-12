package entry

import (
	"testing"
	"time"
)

// An Entry built by hand, without Reset, must not claim that its first field has a role.
func TestZeroEntryHasNoRoles(t *testing.T) {
	var e Entry
	if e.TimeField() != -1 || e.LevelField() != -1 || e.MsgField() != -1 {
		t.Errorf("role indexes = %d, %d, %d; want -1 for all", e.TimeField(), e.LevelField(), e.MsgField())
	}
}

func TestEntryRoles(t *testing.T) {
	var e Entry
	e.SetTimeField(0)
	e.SetLevelField(1)
	e.SetMsgField(2)
	if e.TimeField() != 0 || e.LevelField() != 1 || e.MsgField() != 2 {
		t.Errorf("role indexes = %d, %d, %d; want 0, 1, 2", e.TimeField(), e.LevelField(), e.MsgField())
	}

	// Any negative index clears the role and reads back as -1.
	e.SetTimeField(-1)
	e.SetLevelField(-5)
	e.SetMsgField(-100)
	if e.TimeField() != -1 || e.LevelField() != -1 || e.MsgField() != -1 {
		t.Errorf("after clearing, role indexes = %d, %d, %d; want -1 for all", e.TimeField(), e.LevelField(), e.MsgField())
	}
}

func TestEntryReset(t *testing.T) {
	e := Entry{
		Line:   []byte(`{"level":"info"}`),
		Prefix: []byte("svc |"),
		Format: FormatJSON,
		Time:   time.Unix(1, 0),
		Level:  LevelInfo,
		Fields: make([]Field, 3, 8),

		CollectorPrefix: true,
	}
	e.Fields[0].Key = []byte("level")
	e.SetTimeField(0)
	e.SetLevelField(1)
	e.SetMsgField(2)

	e.Reset()

	if e.Line != nil || e.Prefix != nil || e.CollectorPrefix || e.Format != FormatPlain || !e.Time.IsZero() || e.Level != LevelUnknown {
		t.Errorf("Reset left data behind: %+v", e)
	}
	if e.TimeField() != -1 || e.LevelField() != -1 || e.MsgField() != -1 {
		t.Errorf("role indexes = %d, %d, %d; want -1 for all", e.TimeField(), e.LevelField(), e.MsgField())
	}
	if len(e.Fields) != 0 || cap(e.Fields) != 8 {
		t.Errorf("Fields has len %d and cap %d, want 0 and 8", len(e.Fields), cap(e.Fields))
	}
}

func TestEntryResetDoesNotAllocate(t *testing.T) {
	e := Entry{Fields: make([]Field, 0, 8)}
	allocs := testing.AllocsPerRun(100, func() {
		e.Reset()
		e.Fields = append(e.Fields, Field{}, Field{})
	})
	if allocs != 0 {
		t.Errorf("Reset and reuse allocates %v times per call, want 0", allocs)
	}
}

func TestFormatString(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{FormatPlain, "plain"},
		{FormatJSON, "json"},
		{FormatLogfmt, "logfmt"},
		{Format(200), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.format.String(); got != tt.want {
			t.Errorf("Format(%d).String() = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestKindString(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{KindNone, "none"},
		{KindString, "string"},
		{KindNumber, "number"},
		{KindBool, "bool"},
		{KindNull, "null"},
		{KindObject, "object"},
		{KindArray, "array"},
		{Kind(200), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
