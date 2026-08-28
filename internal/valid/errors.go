package valid

import "errors"

// errorsAs isole l'usage de errors.As, pour que IsValidation reste lisible.
func errorsAs(err error, target any) bool { return errors.As(err, target) }
