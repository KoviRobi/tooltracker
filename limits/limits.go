// Limits which really should be user configurable but I haven't yet made them
// so. Perhaps look into cobra/viper for easier config flags?
package limits

import "time"

const MaxMessageBytes = 1024 * 1024
const MaxRecipients = 10
const WriteTimeout = 10 * time.Second
const ReadTimeout = 10 * time.Second
