// Package ssqcli builds the ssq command tree from the compiled proto service
// descriptors, so every unary RPC is reachable without hand-written glue.
package ssqcli

import (
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	// Registers every session.v1 descriptor in protoregistry.GlobalFiles.
	_ "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

const protoPackage = "session.v1"

// Method is one unary RPC exposed as a command.
type Method struct {
	Group     string // service name without "Service", kebab-case
	Name      string // rpc name, kebab-case
	Procedure string // "/session.v1.SessionService/ListSessions"
	Input     protoreflect.MessageDescriptor
}

// DiscoverMethods lists the unary RPCs of every service in the session.v1
// package, sorted by group then name. Streaming RPCs are omitted: the Connect
// JSON unary transport cannot carry them.
func DiscoverMethods() []Method {
	var out []Method
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if string(fd.Package()) != protoPackage {
			return true
		}
		services := fd.Services()
		for i := 0; i < services.Len(); i++ {
			out = append(out, serviceMethods(services.Get(i))...)
		}
		return true
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func serviceMethods(svc protoreflect.ServiceDescriptor) []Method {
	group := kebab(strings.ReplaceAll(strings.TrimSuffix(string(svc.Name()), "Service"), "GitHub", "Github"))
	var out []Method
	methods := svc.Methods()
	for i := 0; i < methods.Len(); i++ {
		m := methods.Get(i)
		if m.IsStreamingClient() || m.IsStreamingServer() {
			continue
		}
		out = append(out, Method{
			Group:     group,
			Name:      kebab(string(m.Name())),
			Procedure: "/" + string(svc.FullName()) + "/" + string(m.Name()),
			Input:     m.Input(),
		})
	}
	return out
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

// kebab turns "ListSessions" into "list-sessions" and "GetVCSStatus" into "get-vcs-status".
func kebab(s string) string {
	var b strings.Builder
	r := []rune(s)
	for i, c := range r {
		if isUpper(c) && i > 0 {
			prevLower := !isUpper(r[i-1])
			acronymEnd := isUpper(r[i-1]) && i+1 < len(r) && !isUpper(r[i+1])
			if prevLower || acronymEnd {
				b.WriteByte('-')
			}
		}
		if isUpper(c) {
			c += 'a' - 'A'
		}
		b.WriteRune(c)
	}
	return b.String()
}
