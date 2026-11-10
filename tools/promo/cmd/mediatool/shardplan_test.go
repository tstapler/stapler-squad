package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanShards_RoundRobinIsDeterministicAndPartitions(t *testing.T) {
	t.Parallel()
	ids := []string{"d", "b", "a", "c", "e"}
	m := planShards(ids, 2)
	assert.Equal(t, []string{"a", "c", "e"}, m.Include[0].Tours)
	assert.Equal(t, []string{"b", "d"}, m.Include[1].Tours)
	assert.Equal(t, "a,c,e", m.Include[0].Only)
	assert.Equal(t, m, planShards([]string{"e", "c", "a", "b", "d"}, 2), "input order must not matter")
}

func TestPlanShards_EachIdInExactlyOneShard(t *testing.T) {
	t.Parallel()
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "a"}
	m := planShards(ids, 3)
	count := map[string]int{}
	for _, s := range m.Include {
		for _, id := range s.Tours {
			count[id]++
		}
	}
	assert.Len(t, count, 7)
	for id, c := range count {
		assert.Equal(t, 1, c, id)
	}
}

func TestPlanShards_FewerIdsThanShardsLeavesEmptyShards(t *testing.T) {
	t.Parallel()
	m := planShards([]string{"a"}, 3)
	assert.Equal(t, []string{"a"}, m.Include[0].Tours)
	assert.Equal(t, []string{}, m.Include[1].Tours)
}

func TestShardPlan_AddingATourFileTouchesNothingElse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, f := range []string{"a.ts", "b.ts", "_shared.ts", "a.test.ts", "notes.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), nil, 0o644))
	}
	plan := func() shardMatrix {
		var out, errb bytes.Buffer
		require.Equal(t, 0, run([]string{"shard-plan", "-tours-dir", dir, "-shards", "2"}, &out, &errb, os.Getenv), errb.String())
		var m shardMatrix
		require.NoError(t, json.Unmarshal(out.Bytes(), &m))
		return m
	}
	before := plan()
	assert.Equal(t, []string{"a"}, before.Include[0].Tours)
	assert.Equal(t, []string{"b"}, before.Include[1].Tours)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zz-fixture.ts"), nil, 0o644))
	after := plan()
	seen := 0
	for _, s := range after.Include {
		for _, id := range s.Tours {
			if id == "zz-fixture" {
				seen++
			}
		}
	}
	assert.Equal(t, 1, seen)
}

func TestShardPlan_Usage(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	assert.Equal(t, 2, run([]string{"shard-plan", "-shards", "0"}, &out, &errb, os.Getenv))
	assert.Equal(t, 2, run([]string{"shard-plan", "-tours-dir", "/nonexistent-dir"}, &out, &errb, os.Getenv))
}
