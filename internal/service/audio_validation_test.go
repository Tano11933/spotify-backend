package service

import (
	"encoding/binary"
	"testing"
)

func TestAudioValidation(t *testing.T) {
	wav := make([]byte, 48)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], 40)
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 16000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 4)
	mp3 := make([]byte, 417*2)
	copy(mp3, []byte{0xff, 0xfb, 0x90, 0})
	copy(mp3[417:], []byte{0xff, 0xfb, 0x90, 0})
	compressedWAV := append([]byte(nil), wav...)
	compressedWAV[20] = 3
	badAlign := append([]byte(nil), wav...)
	badAlign[32] = 0
	for _, tc := range []struct {
		name, ext string
		data      []byte
		valid     bool
	}{
		{"PCM", ".wav", wav, true}, {"MP3 framing", ".mp3", mp3, true},
		{"MP3 with ID3", ".mp3", append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}, mp3...), true},
		{"fake", ".mp3", []byte("hello"), false}, {"wrong extension", ".mp3", wav, false},
		{"unsupported", ".ogg", mp3, false}, {"truncated WAV", ".wav", wav[:45], false},
		{"float WAV", ".wav", compressedWAV, false}, {"bad alignment", ".wav", badAlign, false},
		{"truncated MP3", ".mp3", mp3[:500], false}, {"ID3 only", ".mp3", []byte("ID3"), false},
		{"no samples", ".wav", wav[:44], false}, {"empty", ".wav", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validAudio(tc.data, tc.ext); got != tc.valid {
				t.Fatalf("valid = %v, want %v", got, tc.valid)
			}
		})
	}
}
