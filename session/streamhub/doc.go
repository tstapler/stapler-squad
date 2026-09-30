// Package streamhub provides the single-owner terminal output hub: one
// StreamHub per tmux session fans output out to N attached Subscribers over
// a Transport-agnostic interface, and is the sole caller of that session's
// resize/quiescence/capture-pane surface, eliminating the multi-connection
// resize/capture race that per-connection ownership allowed.
package streamhub
