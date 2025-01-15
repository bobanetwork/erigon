package engine_helpers

import "github.com/erigontech/erigon/rpc"

const MaxBuilders = 128

var UnknownPayloadErr = rpc.CustomError{Code: -38001, Message: "Unknown payload"}
var InvalidForkchoiceStateErr = rpc.CustomError{Code: -38002, Message: "Invalid forkchoice state"}
var InvalidPayloadAttributesErr = rpc.CustomError{Code: -38003, Message: "Invalid payload attributes"}
var InvalidPayloadAttributesGasLmitErr = rpc.CustomError{Code: -38003, Message: "Invalid payload attributes: gas limit"}
var InvalidPayloadAttributesEIP1559Err = rpc.CustomError{Code: -38003, Message: "Invalid payload attributes: eip155Params not supported prior to Holocene upgrade"}
var TooLargeRequestErr = rpc.CustomError{Code: -38004, Message: "Too large request"}
