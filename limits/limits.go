// Limits which really should be user configurable but I haven't yet made them
// so. Perhaps look into cobra/viper for easier config flags?
package limits

import "time"

var MaxMessageBytes uint32
var MaxRecipients uint32
var WriteTimeout time.Duration
var ReadTimeout time.Duration
