module fixturegen

go 1.21

require (
	buntdbverify v0.0.0
	github.com/tidwall/buntdb v1.3.2
)

require (
	github.com/tidwall/btree v1.4.2 // indirect
	github.com/tidwall/gjson v1.14.3 // indirect
	github.com/tidwall/grect v0.1.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tidwall/rtred v0.1.2 // indirect
	github.com/tidwall/tinyqueue v0.1.1 // indirect
)

replace buntdbverify => ../

// Fixtures must come from the unmodified pinned code.
replace github.com/tidwall/buntdb => ../../upstream/buntdb
