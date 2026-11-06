package terminalguard

// steerAuthorization stands in for the real token: its one defining file is this one.
type steerAuthorization struct {
	kind uint8
	uuid string
}

// rawAuth shares steerAuthorization's underlying type, so a conversion compiles.
type rawAuth struct {
	kind uint8
	uuid string
}

// newSteerAuthorization is the sanctioned constructor, in the defining file.
func newSteerAuthorization() steerAuthorization { return steerAuthorization{kind: 1, uuid: "x"} }
