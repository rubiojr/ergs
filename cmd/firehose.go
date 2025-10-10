package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/urfave/cli/v3"
)

// FirehoseCommand creates a CLI command that tails the warehouse event bridge
// Unix domain socket and writes NDJSON block events to stdout.
//
// Typical usage:
//
//	ergs firehose --socket /run/ergs/bridge.sock
//	ergs firehose                       (uses event_socket_path from config)
//	ergs firehose | jq -r 'select(.type=="block") | .text'
//
// By default it filters to only "block" events and reprints them as-is (single line JSON).
// You can choose to include heartbeats/info/error frames with --all.
// You can request pretty formatting (multi-line) with --pretty (mainly for manual inspection).
//
// The command auto-reconnects with exponential backoff if the socket is not
// yet available or the connection drops. It never exits unless:
//   - Context is cancelled (Ctrl+C / signal)
//   - A non-recoverable error occurs opening the socket AND --no-retry is set.
func FirehoseCommand() *cli.Command {
	return &cli.Command{
		Name:  "firehose",
		Usage: "Stream realtime block events (NDJSON) from the warehouse event bridge socket",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "socket",
				Usage: "Path to Unix domain socket (overrides config event_socket_path)",
			},
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Print all event types (block, heartbeat, info, error) instead of only blocks",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "pretty",
				Usage: "Pretty-print JSON instead of raw single-line",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "no-retry",
				Usage: "Do not retry on failures; exit on first connection error",
				Value: false,
			},
			&cli.DurationFlag{
				Name:  "initial-backoff",
				Usage: "Initial reconnect backoff",
				Value: 1 * time.Second,
			},
			&cli.DurationFlag{
				Name:  "max-backoff",
				Usage: "Maximum reconnect backoff",
				Value: 30 * time.Second,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			cfgPath := c.String("config")
			socketPath := c.String("socket")
			if socketPath == "" {
				// Load config to get default event socket path
				cfg, err := config.LoadConfig(cfgPath)
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				socketPath = cfg.EventSocketPath
			}

			if socketPath == "" {
				return errors.New("no socket path provided (flag --socket or config event_socket_path required)")
			}

			opts := firehoseTailOptions{
				socketPath:     socketPath,
				includeAll:     c.Bool("all"),
				pretty:         c.Bool("pretty"),
				noRetry:        c.Bool("no-retry"),
				initialBackoff: c.Duration("initial-backoff"),
				maxBackoff:     c.Duration("max-backoff"),
				stdout:         os.Stdout,
				stderr:         os.Stderr,
			}
			return tailFirehose(ctx, opts)
		},
	}
}

type firehoseTailOptions struct {
	socketPath     string
	includeAll     bool
	pretty         bool
	noRetry        bool
	initialBackoff time.Duration
	maxBackoff     time.Duration
	stdout         *os.File
	stderr         *os.File
}

func tailFirehose(ctx context.Context, opts firehoseTailOptions) error {
	if opts.initialBackoff <= 0 {
		opts.initialBackoff = time.Second
	}
	if opts.maxBackoff < opts.initialBackoff {
		opts.maxBackoff = 30 * time.Second
	}

	_, _ = fmt.Fprintf(opts.stderr, "Firehose: connecting to %s\n", opts.socketPath)
	backoff := opts.initialBackoff

	for {
		conn, err := net.Dial("unix", opts.socketPath)
		if err != nil {
			if opts.noRetry {
				return fmt.Errorf("dial: %w", err)
			}
			_, _ = fmt.Fprintf(opts.stderr, "Firehose: dial failed (%v), retrying in %s\n", err, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > opts.maxBackoff {
				backoff = opts.maxBackoff
			}
			continue
		}

		_, _ = fmt.Fprintf(opts.stderr, "Firehose: connected (backoff reset)\n")
		backoff = opts.initialBackoff

		if err := streamEvents(ctx, conn, opts); err != nil {
			_ = conn.Close()
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			// Log and attempt reconnect unless no-retry
			if opts.noRetry {
				return err
			}
			_, _ = fmt.Fprintf(opts.stderr, "Firehose: stream error (%v), reconnecting...\n", err)
			// Brief pause before immediate reconnect
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
			continue
		}

		// Normal end (unlikely). Respect no-retry or keep trying.
		if opts.noRetry {
			return nil
		}
		_, _ = fmt.Fprintf(opts.stderr, "Firehose: disconnected, attempting reconnect...\n")
	}
}

func streamEvents(ctx context.Context, conn net.Conn, opts firehoseTailOptions) error {
	defer func() { _ = conn.Close() }()

	// Increase scanner buffer (metadata could be large)
	sc := bufio.NewScanner(conn)
	buf := make([]byte, 64*1024)
	sc.Buffer(buf, 512*1024) // 512KB max line

	for sc.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := sc.Bytes()
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			continue
		}

		// Optimistic fast-path: if user wants raw all events or just blocks?
		// We parse minimally to filter if necessary.
		if !opts.includeAll || opts.pretty {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(line, &raw); err != nil {
				// If malformed, just show raw line when includeAll (else skip)
				if opts.includeAll {
					_, _ = fmt.Fprintln(opts.stdout, trimmed)
				}
				continue
			}

			// Filter if only blocks
			if !opts.includeAll {
				tt, ok := raw["type"]
				if !ok {
					continue
				}
				var t string
				if err := json.Unmarshal(tt, &t); err != nil || t != "block" {
					continue
				}
			}

			if opts.pretty {
				var anyJSON any
				if err := json.Unmarshal(line, &anyJSON); err != nil {
					// Fallback: raw
					_, _ = fmt.Fprintln(opts.stdout, trimmed)
					continue
				}
				b, err := json.MarshalIndent(anyJSON, "", "  ")
				if err != nil {
					_, _ = fmt.Fprintln(opts.stdout, trimmed)
					continue
				}
				_, _ = fmt.Fprintln(opts.stdout, string(b))
				continue
			}
		}

		// Default pass-through (already filtered if needed)
		_, _ = fmt.Fprintln(opts.stdout, trimmed)
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("read error: %w", err)
	}
	return nil
}
