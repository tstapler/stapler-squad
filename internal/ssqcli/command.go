package ssqcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/tstapler/stapler-squad/pkg/localtoken"
)

const (
	defaultURL     = "http://localhost:8543"
	defaultTimeout = 15 * time.Second
)

// Env is everything the CLI reads from its surroundings, injectable for tests.
type Env struct {
	Getenv    func(string) string
	ConfigDir func() (string, error) // local state dir holding the API token
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
}

// DefaultEnv wires Env to the real process.
func DefaultEnv(configDir func() (string, error)) Env {
	return Env{Getenv: os.Getenv, ConfigDir: configDir, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
}

type globals struct {
	url      string
	token    string
	timeout  time.Duration
	jsonBody string
	compact  bool
}

func isReservedFlag(name string) bool {
	switch name {
	case "url", "token", "timeout", "json", "compact", "help":
		return true
	}
	return false
}

// NewRootCommand builds the full ssq command tree from the proto descriptors.
func NewRootCommand(env Env) *cobra.Command {
	g := &globals{}
	root := &cobra.Command{
		Use:           "ssq",
		Short:         "Command line client for the stapler-squad API",
		Long:          "Every unary RPC of the session.v1 services is a command: ssq <service> <rpc> [flags].\nRequest fields become flags; --json sends a raw request body. Aliases live under `ssq sessions`.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)
	pf := root.PersistentFlags()
	urlDefault := env.Getenv("STAPLER_SQUAD_URL")
	if urlDefault == "" {
		urlDefault = defaultURL
	}
	pf.StringVar(&g.url, "url", urlDefault, "server base URL (env STAPLER_SQUAD_URL)")
	pf.StringVar(&g.token, "token", "", "Bearer token (env STAPLER_SQUAD_TOKEN; default: local token file)")
	pf.DurationVar(&g.timeout, "timeout", defaultTimeout, "give up on the RPC after this long")
	pf.StringVar(&g.jsonBody, "json", "", "raw JSON request body, @file, or - for stdin; flags override its fields")
	pf.BoolVar(&g.compact, "compact", false, "print compact single-line JSON")

	methods := DiscoverMethods()
	groups := map[string]*cobra.Command{}
	byProcedure := map[string]Method{}
	for _, m := range methods {
		byProcedure[m.Procedure] = m
		grp := groups[m.Group]
		if grp == nil {
			grp = &cobra.Command{Use: m.Group, Short: "RPCs of the " + m.Group + " service"}
			groups[m.Group] = grp
			root.AddCommand(grp)
		}
		grp.AddCommand(methodCommand(env, g, m, cmdSpec{use: m.Name, short: "Call " + m.Procedure}))
	}
	root.AddCommand(aliasGroup(env, g, byProcedure))
	return root
}

// cmdSpec is the presentation of one RPC command; positional names the
// request field filled from the first argument ("" for none).
type cmdSpec struct {
	use        string
	short      string
	positional string
}

// request carries what buildBody needs to turn flags and args into a message.
type request struct {
	input      protoreflect.MessageDescriptor
	bound      map[string]protoreflect.FieldDescriptor
	positional string
}

func methodCommand(env Env, g *globals, m Method, spec cmdSpec) *cobra.Command {
	maxArgs := 0
	if spec.positional != "" {
		maxArgs = 1
	}
	cmd := &cobra.Command{Use: spec.use, Short: spec.short, Args: cobra.MaximumNArgs(maxArgs)}
	req := request{input: m.Input, bound: bindFields(cmd, m.Input, spec.positional), positional: spec.positional}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		body, err := buildBody(env, g, c, req, args)
		if err != nil {
			return err
		}
		client := &Client{BaseURL: g.url, Token: resolveToken(env, g), Timeout: g.timeout}
		reply, err := client.Call(context.Background(), m.Procedure, body)
		if err != nil {
			return err
		}
		return printJSON(env.Stdout, reply, g.compact)
	}
	return cmd
}

// bindFields registers one flag per scalar (or repeated scalar) request field
// and returns the field descriptors by flag name. Message and map fields are
// reachable through --json only. The positional field is not also a flag.
func bindFields(cmd *cobra.Command, md protoreflect.MessageDescriptor, positional string) map[string]protoreflect.FieldDescriptor {
	bound := map[string]protoreflect.FieldDescriptor{}
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		name := strings.ReplaceAll(string(fd.Name()), "_", "-")
		if isReservedFlag(name) || string(fd.Name()) == positional || fd.IsMap() || !bindFlag(cmd, name, fd) {
			continue
		}
		bound[name] = fd
	}
	return bound
}

func bindFlag(cmd *cobra.Command, name string, fd protoreflect.FieldDescriptor) bool {
	usage := "request field " + string(fd.Name())
	f := cmd.Flags()
	if fd.IsList() {
		switch fd.Kind() { //nolint:exhaustive // remaining kinds are reachable via --json only
		case protoreflect.StringKind, protoreflect.EnumKind:
			f.StringSlice(name, nil, usage)
			return true
		}
		return false
	}
	switch fd.Kind() { //nolint:exhaustive // remaining kinds are reachable via --json only
	case protoreflect.StringKind, protoreflect.EnumKind:
		f.String(name, "", usage)
	case protoreflect.BoolKind:
		f.Bool(name, false, usage)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		f.Int64(name, 0, usage)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind, protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		f.Uint64(name, 0, usage)
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		f.Float64(name, 0, usage)
	default:
		return false
	}
	return true
}

func buildBody(env Env, g *globals, c *cobra.Command, req request, args []string) ([]byte, error) {
	md, positional := req.input, req.positional
	msg := dynamicpb.NewMessage(md)
	if g.jsonBody != "" {
		raw, err := readJSONArg(env, g.jsonBody)
		if err != nil {
			return nil, err
		}
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return nil, fmt.Errorf("--json does not match %s: %w", md.FullName(), err)
		}
	}
	if positional != "" {
		if len(args) == 0 {
			if !msg.Has(md.Fields().ByName(protoreflect.Name(positional))) {
				return nil, fmt.Errorf("missing <%s> argument", positional)
			}
		} else if err := setFromString(msg, md.Fields().ByName(protoreflect.Name(positional)), args[0]); err != nil {
			return nil, err
		}
	}
	for name, fd := range req.bound {
		if !c.Flags().Changed(name) {
			continue
		}
		if err := setFromFlag(msg, fd, c, name); err != nil {
			return nil, fmt.Errorf("--%s: %w", name, err)
		}
	}
	return protojson.Marshal(msg)
}

func setFromFlag(msg *dynamicpb.Message, fd protoreflect.FieldDescriptor, c *cobra.Command, name string) error {
	f := c.Flags()
	if fd.IsList() {
		vals, err := f.GetStringSlice(name)
		if err != nil {
			return err
		}
		list := msg.Mutable(fd).List()
		list.Truncate(0)
		for _, v := range vals {
			pv, err := parseScalar(fd, v)
			if err != nil {
				return err
			}
			list.Append(pv)
		}
		return nil
	}
	switch fd.Kind() { //nolint:exhaustive // remaining kinds are reachable via --json only
	case protoreflect.BoolKind:
		v, _ := f.GetBool(name)
		msg.Set(fd, protoreflect.ValueOfBool(v))
	case protoreflect.StringKind, protoreflect.EnumKind:
		v, _ := f.GetString(name)
		return setFromString(msg, fd, v)
	default:
		return setNumeric(msg, fd, c, name)
	}
	return nil
}

func setNumeric(msg *dynamicpb.Message, fd protoreflect.FieldDescriptor, c *cobra.Command, name string) error {
	f := c.Flags()
	switch fd.Kind() { //nolint:exhaustive // remaining kinds are reachable via --json only
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		v, _ := f.GetInt64(name)
		msg.Set(fd, protoreflect.ValueOfInt32(int32(v)))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		v, _ := f.GetInt64(name)
		msg.Set(fd, protoreflect.ValueOfInt64(v))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		v, _ := f.GetUint64(name)
		msg.Set(fd, protoreflect.ValueOfUint32(uint32(v)))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		v, _ := f.GetUint64(name)
		msg.Set(fd, protoreflect.ValueOfUint64(v))
	case protoreflect.FloatKind:
		v, _ := f.GetFloat64(name)
		msg.Set(fd, protoreflect.ValueOfFloat32(float32(v)))
	case protoreflect.DoubleKind:
		v, _ := f.GetFloat64(name)
		msg.Set(fd, protoreflect.ValueOfFloat64(v))
	default:
		return fmt.Errorf("unsupported field kind %s", fd.Kind())
	}
	return nil
}

func setFromString(msg *dynamicpb.Message, fd protoreflect.FieldDescriptor, s string) error {
	pv, err := parseScalar(fd, s)
	if err != nil {
		return err
	}
	msg.Set(fd, pv)
	return nil
}

// parseScalar parses a string or enum field value; enums accept the name
// (case-insensitive, with or without the type prefix) or the number.
func parseScalar(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
	if fd.Kind() == protoreflect.StringKind {
		return protoreflect.ValueOfString(s), nil
	}
	if fd.Kind() != protoreflect.EnumKind {
		return protoreflect.Value{}, fmt.Errorf("unsupported field kind %s", fd.Kind())
	}
	values := fd.Enum().Values()
	want := strings.ToUpper(s)
	for i := 0; i < values.Len(); i++ {
		v := values.Get(i)
		name := string(v.Name())
		if name == want || strings.HasSuffix(name, "_"+want) {
			return protoreflect.ValueOfEnum(v.Number()), nil
		}
	}
	var n int32
	if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
		return protoreflect.ValueOfEnum(protoreflect.EnumNumber(n)), nil
	}
	return protoreflect.Value{}, fmt.Errorf("%q is not a value of %s", s, fd.Enum().Name())
}

func readJSONArg(env Env, arg string) ([]byte, error) {
	switch {
	case arg == "-":
		return io.ReadAll(env.Stdin)
	case strings.HasPrefix(arg, "@"):
		return os.ReadFile(strings.TrimPrefix(arg, "@"))
	default:
		return []byte(arg), nil
	}
}

// resolveToken: --token, then STAPLER_SQUAD_TOKEN, then the local token file.
func resolveToken(env Env, g *globals) string {
	if g.token != "" {
		return g.token
	}
	if t := env.Getenv("STAPLER_SQUAD_TOKEN"); t != "" {
		return t
	}
	if env.ConfigDir == nil {
		return ""
	}
	dir, err := env.ConfigDir()
	if err != nil {
		return ""
	}
	return localtoken.FromConfigDir(dir)
}

func printJSON(w io.Writer, raw []byte, compact bool) error {
	var buf bytes.Buffer
	indent := "  "
	if compact {
		indent = ""
	}
	if err := indentJSON(&buf, raw, indent); err != nil {
		buf.Reset()
		buf.Write(raw)
	}
	buf.WriteByte('\n')
	_, werr := w.Write(buf.Bytes())
	return werr
}

func indentJSON(buf *bytes.Buffer, raw []byte, indent string) error {
	if indent == "" {
		return json.Compact(buf, raw)
	}
	return json.Indent(buf, raw, "", indent)
}
