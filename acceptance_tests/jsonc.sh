#!/bin/bash

setUp() {
  rm test*.jsonc 2>/dev/null || true
}

testJsoncIssueExample() {
  cat >data1.jsonc <<EOL
// sing-box 1.12.0 migration
{
  "dns": {
    "servers": [
      {
        "address": "local",
      }
    ]
  }
}
EOL

  read -r -d '' expected << EOM
// sing-box 1.12.0 migration
{
  "dns": {
    "servers": [
      {
        "address": "local",
        "type": "local"
      }
    ]
  }
}
EOM

  X=$(./yq -P -o jsonc -p jsonc '.dns.servers[0].type = .dns.servers[0].address' data1.jsonc)
  assertEquals "$expected" "$X"

  rm data1.jsonc
}

testJsoncExtensionAutoDetected() {
  cat >test.jsonc <<EOL
{
  // a comment
  "a": 1,
}
EOL

  read -r -d '' expected << EOM
{
  // a comment
  "a": 1
}
EOM

  X=$(./yq -P '.' test.jsonc)
  assertEquals "$expected" "$X"
}

testJsoncOutputFormat() {
  cat >test.jsonc <<EOL
{"a": 1}
EOL

  X=$(./yq -o=jsonc -P '.' test.jsonc)
  assertEquals "{
  \"a\": 1
}" "$X"
}

testJsoncDoesNotAffectPlainJsonFormat() {
  cat >test.jsonc <<EOL
{ "a": 1, /* comment */ }
EOL

  X=$(./yq -p=json '.' test.jsonc 2>&1)
  assertNotEquals 0 $?
}

testJsoncDoubleTrailingCommaErrors() {
  cat >test.jsonc <<EOL
[1, 2,,]
EOL

  ./yq -p=jsonc '.' test.jsonc >/dev/null 2>&1
  assertEquals 1 $?
}

source ./scripts/shunit2
