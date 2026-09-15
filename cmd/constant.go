package cmd

var unwrapScalarFlag = newUnwrapFlag()

var printNodeInfo = false

var unwrapScalar = false

var writeInplace = false
var outputToJSON = false

var outputFormat = ""

var inputFormat = ""

// filename to use for auto format detection when reading from stdin;
// purely a string hint - never resolved against the filesystem.
var stdinFilename = ""

var exitStatus = false
var indent = 2
var noDocSeparators = false
var nullInput = false
var nulSepOutput = false
var verbose = false
var version = false
var prettyPrint = false

var forceColor = false
var forceNoColor = false
var colorsEnabled = false

// can be either "" (off), "extract" or "process"
var frontMatter = ""

var splitFileExp = ""
var splitFileExpFile = ""

var completedSuccessfully = false

var forceExpression = ""

var expressionFile = ""
