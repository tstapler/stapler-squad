# Spike: FSEvents event volume — NOT YET MEASURED

Authored on a Linux host; FSEvents needs macOS, so no numbers exist yet. Do not
cite figures from this file until the table below is filled in.

Run on macOS from the repo root (harness: `internal/fsevents/spike_darwin_test.go`):

```
FSEVENTS_SPIKE_REPO=$PWD FSEVENTS_SPIKE_CMD='go build .' \
  go test ./internal/fsevents -run SpikeEventVolume -v
FSEVENTS_SPIKE_REPO=$PWD FSEVENTS_SPIKE_CMD='git checkout <other-branch> && git checkout -' \
  go test ./internal/fsevents -run SpikeEventVolume -v
```

| Workload | events | rescan-flag events | elapsed | events/s |
|---|---|---|---|---|
| `go build .` | | | | |
| `git checkout` | | | | |
