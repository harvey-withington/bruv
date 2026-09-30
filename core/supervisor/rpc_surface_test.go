package supervisor

import (
	"reflect"
	"sort"
	"testing"
)

// internalRuntimeMethods are exported *Runtime methods that must NOT be
// reachable over /repos/<id>/rpc: lifecycle, wiring for the transport
// and desktop shell, and the boardtools.Board / native-tool plumbing.
var internalRuntimeMethods = []string{
	"Close", "RPCMethods",
	"AgentRT", "AgentService", "Bus", "CardService", "CatalogService",
	"ChatRT", "Index", "LLMActors", "MCPRegistry", "ProjectService",
	"Prompts", "Repo", "SchemaRegistry", "Tools", "Watcher",
	"ResolveAttachment", "WorkspaceGitServeDir", "PresentCardJSON",
	"ListCategoryCards", "ReadCardAttachment",
	"SchemaBlocks",
}

// internalMachineMethods are exported *MachineService methods that
// must NOT be reachable over /server/rpc.
var internalMachineMethods = []string{"RPCMethods", "WithPush"}

// TestRPCSurfacePinned: every exported method is classified as RPC or
// internal, exactly once, and every listed name exists. A new exported
// method fails this test until someone decides which it is — before
// this pin, reflection exposed all of them (Close, WithPush, Bus, …)
// to any device token.
func TestRPCSurfacePinned(t *testing.T) {
	cases := []struct {
		name     string
		typ      reflect.Type
		rpc      []string
		internal []string
	}{
		{"Runtime", reflect.TypeOf(&Runtime{}), repoRPCMethods, internalRuntimeMethods},
		{"MachineService", reflect.TypeOf(&MachineService{}), machineRPCMethods, internalMachineMethods},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class := map[string]string{}
			for _, n := range tc.rpc {
				if class[n] != "" {
					t.Errorf("%s listed twice", n)
				}
				class[n] = "rpc"
			}
			for _, n := range tc.internal {
				if class[n] != "" {
					t.Errorf("%s listed as both rpc and internal (or twice)", n)
				}
				class[n] = "internal"
			}
			exported := map[string]bool{}
			var unclassified []string
			for i := 0; i < tc.typ.NumMethod(); i++ {
				n := tc.typ.Method(i).Name
				exported[n] = true
				if class[n] == "" {
					unclassified = append(unclassified, n)
				}
			}
			sort.Strings(unclassified)
			if len(unclassified) > 0 {
				t.Errorf("exported methods not classified as RPC or internal: %v", unclassified)
			}
			for n := range class {
				if !exported[n] {
					t.Errorf("%s is listed but is not a method of %s", n, tc.name)
				}
			}
		})
	}
}
