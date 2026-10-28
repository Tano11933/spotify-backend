package service

import (
	"bytes"
	"encoding/binary"
)

// Validation checks framing and PCM layout; it does not decode compressed samples.
func validAudio(data []byte, ext string) bool {
	switch ext {
	case ".wav":
		return validPCM(data)
	case ".mp3":
		return validMP3(data)
	default:
		return false
	}
}

func validPCM(data []byte) bool {
	if len(data) < 44 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" ||
		uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return false
	}
	format, samples := false, false
	var alignment uint16
	for offset := 12; offset < len(data); {
		if offset+8 > len(data) {
			return false
		}
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		start := offset + 8
		if size > len(data)-start {
			return false
		}
		chunk := data[start : start+size]
		switch string(data[offset : offset+4]) {
		case "fmt ":
			if format || len(chunk) < 16 {
				return false
			}
			channels := binary.LittleEndian.Uint16(chunk[2:4])
			rate := binary.LittleEndian.Uint32(chunk[4:8])
			bits := binary.LittleEndian.Uint16(chunk[14:16])
			alignment = channels * bits / 8
			if binary.LittleEndian.Uint16(chunk[:2]) != 1 || (channels != 1 && channels != 2) ||
				(bits != 8 && bits != 16) || rate < 8000 || rate > 192000 ||
				binary.LittleEndian.Uint16(chunk[12:14]) != alignment ||
				binary.LittleEndian.Uint32(chunk[8:12]) != rate*uint32(alignment) {
				return false
			}
			format = true
		case "data":
			if !format || samples || size == 0 || size%int(alignment) != 0 {
				return false
			}
			samples = true
		}
		offset = start + size + size%2
		if offset > len(data) {
			return false
		}
	}
	return format && samples
}

func validMP3(data []byte) bool {
	if bytes.HasPrefix(data, []byte("ID3")) {
		if len(data) < 10 || data[3] < 2 || data[3] > 4 {
			return false
		}
		size := 0
		for _, b := range data[6:10] {
			if b >= 128 {
				return false
			}
			size = size<<7 | int(b)
		}
		end := 10 + size
		if data[3] == 4 && data[5]&0x10 != 0 {
			end += 10
		}
		if end > len(data) {
			return false
		}
		data = data[end:]
	}
	if len(data) >= 128 && string(data[len(data)-128:len(data)-125]) == "TAG" {
		data = data[:len(data)-128]
	}
	frames := 0
	for len(data) > 0 {
		if len(data) < 4 || data[0] != 0xff || data[1]&0xe0 != 0xe0 {
			return false
		}
		version, layer := (data[1]>>3)&3, (data[1]>>1)&3
		bitrate, sample := (data[2]>>4)&15, (data[2]>>2)&3
		if version == 1 || layer != 1 || bitrate == 0 || bitrate == 15 || sample == 3 {
			return false
		}
		rates := [3]int{44100, 48000, 32000}
		rate := rates[sample]
		bitrates := [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
		factor := 144
		if version != 3 {
			bitrates = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
			factor = 72
			rate /= 2
			if version == 0 {
				rate /= 2
			}
		}
		size := factor*bitrates[bitrate]*1000/rate + int((data[2]>>1)&1)
		if size < 4 || size > len(data) {
			return false
		}
		data = data[size:]
		frames++
	}
	return frames >= 2
}
