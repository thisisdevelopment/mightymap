module migrate-badger

go 1.21

require (
	gopkg.in/yaml.v3 v3.0.1
	github.com/thisisdevelopment/mightymap v0.5.1
	github.com/thisisdevelopment/mightymap-legacy v0.4.5
)

// Replace directives for dual version support
// Point to local directories containing the different versions
replace github.com/thisisdevelopment/mightymap => ../current
replace github.com/thisisdevelopment/mightymap-legacy => ../legacy

require (
	github.com/dgraph-io/badger/v4 v4.2.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
) 