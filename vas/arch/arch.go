// Package arch defines the architecture interface for VAS backends.
//
// Each architecture (x86, ARM, RISC-V, etc.) implements this interface
// to provide register mapping, instruction expansion, and target-specific
// boilerplate generation. This allows VAS to support multiple assembly
// syntaxes with a single binary.
package arch

// Architecture defines the interface that each target backend must implement.
type Architecture interface {
	// Name returns the architecture name (e.g. "x86", "arm64", "riscv64").
	Name() string

	// MapReg replaces a virtual register reference (e.g. "v0") with the
	// corresponding physical register name for this architecture.
	// It should also validate that the virtual register is in range.
	MapReg(s string) (string, error)

	// MaxRegs returns the number of virtual registers supported (v0..v<N-1>).
	MaxRegs() int

	// PhysicalRegs returns the list of physical register names mapped to v0..v<N-1>.
	PhysicalRegs() []string

	// ExpandInstruction translates a single VAS instruction line
	// (opcode + arguments, already register-mapped) into one or more
	// target assembly lines. Returns nil if this architecture does not
	// recognise the opcode (fallback to raw passthrough).
	ExpandInstruction(opcode string, args []string) ([]string, error)

	// WrapStandalone wraps assembled output with the minimal program skeleton
	// (section declarations, entry point, exit code) for this architecture.
	WrapStandalone(vasInput, asmOutput string) string
}

// Registry maps architecture names to their implementations.
var registry = map[string]Architecture{}

// Register adds an architecture to the global registry.
func Register(a Architecture) {
	registry[a.Name()] = a
}

// Get returns the architecture implementation for the given name.
// Returns nil if not found.
func Get(name string) Architecture {
	return registry[name]
}

// List returns all registered architecture names.
func List() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	return names
}
