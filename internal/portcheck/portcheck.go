package portcheck

type HolderKind int

const (
	HolderNone HolderKind = iota
	HolderSAG
	HolderContainer
	HolderProcess
)

type Holder struct {
	Kind HolderKind
	Name string
}
