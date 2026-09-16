//go:build yq_nojson

package yqlib

func NewJSONCDecoder() Decoder {
	return nil
}

func NewJSONCEncoder(prefs JsonPreferences) Encoder {
	return nil
}
