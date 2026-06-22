package vas

import "errors"

var (
	ErrInvalidRegister = errors.New("invalid virtual register (valid: v0-v12)")
	ErrOperandCount    = errors.New("operand count mismatch")
	ErrLintError       = errors.New("lint errors found")
	ErrPreprocessing   = errors.New("preprocessing error")
	ErrUndefinedLabel  = errors.New("undefined label")
)
