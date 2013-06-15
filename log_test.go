package raft

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"
)

func oneshot() chan []byte {
	return make(chan []byte, 1)
}

func noop(uint64, []byte) []byte {
	return []byte{}
}

func nocfg(Peers) error {
	return nil
}

func TestLogEntriesAfter(t *testing.T) {
	c := []byte(`{}`)
	buf := &bytes.Buffer{}
	log := NewLog(buf, noop, nocfg)

	type tuple struct {
		AfterIndex      uint64
		ExpectedEntries int
		ExpectedTerm    uint64
	}

	for _, tu := range []tuple{
		{0, 0, 0},
		{1, 0, 0},
		{2, 0, 0},
		{3, 0, 0},
		{4, 0, 0},
	} {
		entries, term := log.entriesAfter(tu.AfterIndex)
		if expected, got := tu.ExpectedEntries, len(entries); expected != got {
			t.Errorf("with %d, After(%d): entries: expected %d got %d", 0, tu.AfterIndex, expected, got)
		}
		if expected, got := tu.ExpectedTerm, term; expected != got {
			t.Errorf("with %d, After(%d): expected %d got %d", 0, tu.AfterIndex, expected, got)
		}
	}

	log.appendEntry(LogEntry{1, 1, c, oneshot(), false})
	for _, tu := range []tuple{
		{0, 1, 0},
		{1, 0, 1},
		{2, 0, 1},
		{3, 0, 1},
		{4, 0, 1},
	} {
		entries, term := log.entriesAfter(tu.AfterIndex)
		if expected, got := tu.ExpectedEntries, len(entries); expected != got {
			t.Errorf("with %d, After(%d): entries: expected %d got %d", 1, tu.AfterIndex, expected, got)
		}
		if expected, got := tu.ExpectedTerm, term; expected != got {
			t.Errorf("with %d, After(%d): term: expected %d got %d", 1, tu.AfterIndex, expected, got)
		}
	}

	log.appendEntry(LogEntry{2, 1, c, oneshot(), false})
	for _, tu := range []tuple{
		{0, 2, 0},
		{1, 1, 1},
		{2, 0, 1},
		{3, 0, 1},
		{4, 0, 1},
	} {
		entries, term := log.entriesAfter(tu.AfterIndex)
		if expected, got := tu.ExpectedEntries, len(entries); expected != got {
			t.Errorf("with %d, After(%d): entries: expected %d got %d", 2, tu.AfterIndex, expected, got)
		}
		if expected, got := tu.ExpectedTerm, term; expected != got {
			t.Errorf("with %d, After(%d): term: expected %d got %d", 2, tu.AfterIndex, expected, got)
		}
	}

	log.appendEntry(LogEntry{3, 2, c, oneshot(), false})
	for _, tu := range []tuple{
		{0, 3, 0},
		{1, 2, 1},
		{2, 1, 1},
		{3, 0, 2},
		{4, 0, 2},
	} {
		entries, term := log.entriesAfter(tu.AfterIndex)
		if expected, got := tu.ExpectedEntries, len(entries); expected != got {
			t.Errorf("with %d, After(%d): entries: expected %d got %d", 3, tu.AfterIndex, expected, got)
		}
		if expected, got := tu.ExpectedTerm, term; expected != got {
			t.Errorf("with %d, After(%d): term: expected %d got %d", 3, tu.AfterIndex, expected, got)
		}
	}
}

func TestLogEntryEncodeDecode(t *testing.T) {
	for _, logEntry := range []LogEntry{
		LogEntry{1, 1, []byte(`{}`), oneshot(), false},
		LogEntry{1, 2, []byte(`{}`), oneshot(), false},
		LogEntry{1, 2, []byte(`{}`), oneshot(), false},
		LogEntry{2, 2, []byte(`{}`), oneshot(), false},
		LogEntry{255, 3, []byte(`{"cmd": 123}`), oneshot(), false},
		LogEntry{math.MaxUint64 - 1, math.MaxUint64, []byte(`{}`), oneshot(), false},
	} {
		b := &bytes.Buffer{}
		if err := logEntry.encode(b); err != nil {
			t.Errorf("%v: Encode: %s", logEntry, err)
			continue
		}
		t.Logf("%v: Encode: %s", logEntry, strings.TrimSpace(b.String()))

		var e LogEntry
		if err := e.decode(b); err != nil {
			t.Errorf("%v: Decode: %s", logEntry, err)
		}
	}
}

func TestLogAppend(t *testing.T) {
	c := []byte(`{}`)
	buf := &bytes.Buffer{}
	log := NewLog(buf, noop, nocfg)

	// Append 3 valid LogEntries
	if err := log.appendEntry(LogEntry{1, 1, c, oneshot(), false}); err != nil {
		t.Errorf("Append: %s", err)
	}
	if err := log.appendEntry(LogEntry{2, 1, c, oneshot(), false}); err != nil {
		t.Errorf("Append: %s", err)
	}
	if err := log.appendEntry(LogEntry{3, 2, c, oneshot(), false}); err != nil {
		t.Errorf("Append: %s", err)
	}

	// Append some invalid LogEntries
	if err := log.appendEntry(LogEntry{4, 1, c, oneshot(), false}); err != ErrTermTooSmall {
		t.Errorf("Append: expected ErrTermTooSmall, got %v", err)
	}
	if err := log.appendEntry(LogEntry{2, 2, c, oneshot(), false}); err != ErrIndexTooSmall {
		t.Errorf("Append: expected ErrIndexTooSmall, got %v", nil)
	}

	// Check our flush buffer, before doing any commits
	precommit := ``
	if expected, got := precommit, buf.String(); expected != got {
		t.Errorf("before commit, expected:\n%s\ngot:\n%s\n", expected, got)
	}

	// Commit the first two, only
	if err := log.commitTo(2); err != nil {
		t.Fatalf("commitTo: %s", err)
	}

	// Check our flush buffer
	for i, expected := range log.entries[:2] {
		var got LogEntry
		if err := got.decode(buf); err != nil {
			t.Fatalf("after commit, got: %s", err)
		}

		if expected.Term != got.Term {
			t.Errorf("%d. decode expected term=%d, got %d", i, expected.Term, got.Term)
		}

		if expected.Index != got.Index {
			t.Errorf("%d. decode expected index=%d, got %d", i, expected.Index, got.Index)
		}

		if !bytes.Equal(expected.Command, got.Command) {
			t.Errorf("%d. decode expected command=%q, got %q", i, expected.Command, got.Command)
		}
	}

	// Make some invalid commits
	if err := log.commitTo(1); err != ErrIndexTooSmall {
		t.Errorf("Commit: expected ErrIndexTooSmall, got %v", err)
	}
	if err := log.commitTo(4); err != ErrIndexTooBig {
		t.Errorf("Commit: expected ErrIndexTooBig, got %v", err)
	}

	// Commit every LogEntry
	if err := log.commitTo(3); err != nil {
		t.Errorf("commitTo: %s", err)
	}

	// Check our buffer again
	expected := log.entries[2]
	var got LogEntry
	if err := got.decode(buf); err != nil {
		t.Fatalf("after commit, got: %s", err)
	}

	if expected.Term != got.Term {
		t.Errorf("%d. decode expected term=%d, got %d", 3, expected.Term, got.Term)
	}

	if expected.Index != got.Index {
		t.Errorf("%d. decode expected index=%d, got %d", 3, expected.Index, got.Index)
	}

	if !bytes.Equal(expected.Command, got.Command) {
		t.Errorf("%d. decode expected command=%q, got %q", 3, expected.Command, got.Command)
	}
}

func TestLogContains(t *testing.T) {
	c := []byte(`{}`)
	buf := &bytes.Buffer{}
	log := NewLog(buf, noop, nocfg)

	for _, tuple := range []struct {
		Index uint64
		Term  uint64
	}{
		{1, 1},
		{2, 1},
		{3, 2},
	} {
		e := LogEntry{tuple.Index, tuple.Term, c, oneshot(), false}
		if err := log.appendEntry(e); err != nil {
			t.Fatalf("appendEntry(%v): %s", e, err)
		}
	}

	for _, tuple := range []struct {
		Index    uint64
		Term     uint64
		Expected bool
	}{
		{0, 1, false},
		{1, 0, false},
		{1, 1, true},
		{2, 1, true},
		{1, 2, false},
		{3, 2, true},
		{3, 3, false},
		{3, 4, false},
		{4, 3, false},
		{4, 4, false},
		{4, 1, false},
		{1, 4, false},
	} {
		index, term, expected := tuple.Index, tuple.Term, tuple.Expected
		if got := log.contains(index, term); expected != got {
			t.Errorf("Contains(%d, %d): expected %v, got %v", index, term, expected, got)
		}
	}
}

func TestLogTruncation(t *testing.T) {
	c := []byte(`{}`)
	buf := &bytes.Buffer{}
	log := NewLog(buf, noop, nocfg)

	for _, tuple := range []struct {
		Index uint64
		Term  uint64
	}{
		{1, 1},
		{2, 1},
		{3, 2},
	} {
		e := LogEntry{tuple.Index, tuple.Term, c, oneshot(), false}
		if err := log.appendEntry(e); err != nil {
			t.Fatalf("appendEntry(%v): %s", e, err)
		}
	}

	if err := log.commitTo(2); err != nil {
		t.Fatal(err)
	}

	if expected, got := ErrIndexTooBig, log.ensureLastIs(4, 3); expected != got {
		t.Errorf("expected %s, got %v", expected, got)
	}
	if expected, got := ErrIndexTooSmall, log.ensureLastIs(1, 1); expected != got {
		t.Errorf("expected %s, got %v", expected, got) // before commitIndex
	}
	if expected, got := ErrBadTerm, log.ensureLastIs(3, 4); expected != got {
		t.Errorf("expected %s, got %v", expected, got)
	}

	if err := log.ensureLastIs(3, 2); err != nil {
		t.Fatal(err)
	}
	if err := log.ensureLastIs(2, 1); err != nil {
		t.Fatal(err)
	}
	if log.contains(3, 2) {
		t.Fatal("should have truncated (3,2) but it still exists")
	}
	if !log.contains(2, 1) {
		t.Fatal("(2,1) should still exist but it seems to be missing")
	}
}

func TestLogCommitNoDuplicate(t *testing.T) {
	// A pathological case: serial commitTo may double-apply the first command
	hits := 0
	apply := func(uint64, []byte) []byte { hits++; return []byte{} }
	log := NewLog(&bytes.Buffer{}, apply, nocfg)

	log.appendEntry(LogEntry{Index: 1, Term: 1, Command: []byte(`{}`)})
	log.commitTo(1)
	if expected, got := 1, hits; expected != got {
		t.Errorf("expected %d hits, got %d", expected, got)
	}

	log.appendEntry(LogEntry{Index: 2, Term: 1, Command: []byte(`{}`)})
	log.commitTo(2)
	if expected, got := 2, hits; expected != got {
		t.Errorf("expected %d hits, got %d", expected, got)
	}

	log.appendEntry(LogEntry{Index: 3, Term: 1, Command: []byte(`{}`)})
	log.commitTo(3)
	if expected, got := 3, hits; expected != got {
		t.Errorf("expected %d hits, got %d", expected, got)
	}
}

func TestLogCommitTwice(t *testing.T) {
	// A pathological case: commitTo(N) twice in a row should be fine.
	log := NewLog(&bytes.Buffer{}, noop, nocfg)

	log.appendEntry(LogEntry{Index: 1, Term: 1, Command: []byte(`{}`)})
	log.commitTo(1)

	log.appendEntry(LogEntry{Index: 2, Term: 1, Command: []byte(`{}`)})
	log.commitTo(2)
	log.commitTo(2) // shouldn't crash

	if expected, got := uint64(2), log.getCommitIndex(); expected != got {
		t.Errorf("expected commitIndex %d, got %d", expected, got)
	}
}

func TestCleanLogRecovery(t *testing.T) {
	entries := []LogEntry{
		{1, 1, []byte("{}"), nil, false},
		{2, 1, []byte("{}"), nil, false},
		{3, 2, []byte("{}"), nil, false},
	}

	buf := new(bytes.Buffer)
	for _, entry := range entries {
		entry.encode(buf)
	}
	log := NewLog(buf, noop, nocfg)

	if expected, got := len(entries), len(log.entries); expected != got {
		t.Fatalf("expected %d, got %d", expected, got)
	}

	if !log.contains(1, 1) {
		t.Errorf("log doesn't contain index=1 term=1")
	}
	if !log.contains(2, 1) {
		t.Errorf("log doesn't contain index=2 term=1")
	}
	if !log.contains(3, 2) {
		t.Errorf("log doesn't contain index=3 term=2")
	}

	if expected, got := uint64(3), log.getCommitIndex(); expected != got {
		t.Errorf("expected commit index = %d, got %d", expected, got)
	}

	if expected, got := uint64(2), log.lastTerm(); expected != got {
		t.Errorf("expected term = %d, got %d", expected, got)
	}

	log.commitTo(3) // should be a no-op

	if buf.Len() > 0 {
		t.Errorf("commit to recovered index wrote to buffer")
	}

	if err := log.appendEntry(LogEntry{
		Index:   4,
		Term:    3,
		Command: []byte(`{"foo": "bar"}`),
	}); err != nil {
		t.Fatalf("append entry: %s", err)
	}
	if expected, got := 4, len(log.entries); expected != got {
		t.Fatalf("expected %d, got %d", expected, got)
	}
	if !log.contains(4, 3) {
		t.Errorf("log doesn't contain index=4 term=3")
	}
}

func TestCorruptedLogRecovery(t *testing.T) {
	entries := []LogEntry{
		{1, 1, []byte("{}"), nil, false},
	}

	buf := &bytes.Buffer{}
	for _, entry := range entries {
		entry.encode(buf)
	}
	buf.Write([]byte("garbage"))
	log := NewLog(buf, noop, nocfg)

	if expected, got := len(entries), len(log.entries); expected != got {
		t.Fatalf("expected %d, got %d", expected, got)
	}

	if !log.contains(1, 1) {
		t.Errorf("log doesn't contain index=1 term=1")
	}
	if log.contains(2, 1) {
		t.Errorf("log contains corrupted index=2 term=1")
	}
	if log.contains(3, 2) {
		t.Errorf("log contains corrupted index=3 term=2")
	}

	if err := log.appendEntry(LogEntry{
		Index:   4,
		Term:    3,
		Command: []byte(`{"foo": "bar"}`),
	}); err != nil {
		t.Fatalf("append entry: %s", err)
	}
	if expected, got := 2, len(log.entries); expected != got {
		t.Fatalf("expected %d, got %d", expected, got)
	}
	if !log.contains(4, 3) {
		t.Errorf("log doesn't contain index=4 term=3")
	}
}

func TestConfigurationDecode(t *testing.T) {
	expectedPeers := Peers{
		1: serializablePeer{1, "one"},
		2: serializablePeer{2, "two"},
	}

	var i int32
	cfg := func(gotPeers Peers) error {
		for id := range expectedPeers {
			if expected, got := expectedPeers[id], gotPeers[id]; expected != got {
				return fmt.Errorf("cfg: %d: expected %v, got %v", id, expected, got)
			}
		}
		atomic.AddInt32(&i, 1)
		return nil
	}

	buf := &bytes.Buffer{}
	log := NewLog(buf, noop, cfg)

	peersBuf := &bytes.Buffer{}
	gob.Register(serializablePeer{})
	if err := gob.NewEncoder(peersBuf).Encode(expectedPeers); err != nil {
		t.Fatal(err)
	}
	log.appendEntry(LogEntry{
		Index:           1,
		Term:            1,
		Command:         peersBuf.Bytes(),
		isConfiguration: true,
	})
	log.commitTo(log.lastIndex())

	if i != 1 {
		t.Fatalf("'cfg' wasn't called; no check was made")
	}
}

func TestLogConfigurationFlag(t *testing.T) {
	cmd := func(i *int32) func(uint64, []byte) []byte {
		return func(index uint64, cmd []byte) []byte {
			t.Logf("cmd(%d, %v)", index, cmd)
			atomic.AddInt32(i, 1)
			return []byte{}
		}
	}

	cfg := func(i *int32) func(Peers) error {
		return func(peers Peers) error {
			t.Logf("cfg(%v)", peers)
			atomic.AddInt32(i, 1)
			return nil
		}
	}

	buf := &bytes.Buffer{}
	var cmdCount, cfgCount int32
	log := NewLog(buf, cmd(&cmdCount), cfg(&cfgCount))

	log.appendEntry(LogEntry{
		Index:           1,
		Term:            1,
		Command:         []byte(`{}`),
		isConfiguration: false,
	})
	log.commitTo(log.lastIndex())
	if cmdCount != 1 {
		t.Fatalf("command failed to register")
	}
	if cfgCount != 0 {
		t.Fatalf("config improperly incremented")
	}

	peers := Peers{}
	peersBuf := &bytes.Buffer{}
	if err := gob.NewEncoder(peersBuf).Encode(peers); err != nil {
		t.Fatal(err)
	}
	log.appendEntry(LogEntry{
		Index:           2,
		Term:            1,
		Command:         peersBuf.Bytes(),
		isConfiguration: true,
	})
	log.commitTo(log.lastIndex())
	if cmdCount != 1 {
		t.Fatalf("command improperly incremented")
	}
	if cfgCount != 1 {
		t.Fatalf("config failed to register")
	}
}

type serializablePeer struct {
	MyId uint64
	Err  string
}

func (p serializablePeer) Id() uint64 { return p.MyId }
func (p serializablePeer) AppendEntries(AppendEntries) AppendEntriesResponse {
	return AppendEntriesResponse{}
}
func (p serializablePeer) RequestVote(rv RequestVote) RequestVoteResponse {
	return RequestVoteResponse{}
}
func (p serializablePeer) Command([]byte, chan []byte) error {
	return fmt.Errorf("%s", p.Err)
}
func (p serializablePeer) SetConfiguration(Peers) error {
	return fmt.Errorf("%s", p.Err)
}
