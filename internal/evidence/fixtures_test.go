package evidence

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"testing"
)

func pic(w, h int) image.Image {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	m.Set(0, 0, color.RGBA{255, 0, 0, 255})
	return m
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, pic(w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, pic(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gifOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := gif.Encode(&b, pic(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func webpOf(w, h int) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x00\x00\x00\x00")
	b = append(b, byte(w-1), byte((w-1)>>8), byte((w-1)>>16), byte(h-1), byte((h-1)>>8), byte((h-1)>>16))
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	return b
}

func bmffBox(typ string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	b := binary.BigEndian.AppendUint32(nil, uint32(8+len(body)))
	return append(append(b, typ...), body...)
}

// mp4Of is a minimal ISO BMFF file: ftyp, moov (mvhd + one trak with a
// tkhd for a w×h track) and an mdat. brand "qt  " makes it QuickTime.
func mp4Of(brand string, durationMs uint32, w, h int) []byte {
	ftyp := bmffBox("ftyp", []byte(brand), []byte{0, 0, 2, 0}, []byte(brand))
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:], 1000) // timescale
	binary.BigEndian.PutUint32(mvhd[16:], durationMs)
	tkhd := make([]byte, 84)
	binary.BigEndian.PutUint32(tkhd[76:], uint32(w)<<16)
	binary.BigEndian.PutUint32(tkhd[80:], uint32(h)<<16)
	moov := bmffBox("moov", bmffBox("mvhd", mvhd), bmffBox("trak", bmffBox("tkhd", tkhd)))
	return bytes.Join([][]byte{ftyp, moov, bmffBox("mdat", []byte("frames"))}, nil)
}

func ebml(id uint32, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	var idb []byte
	switch {
	case id > 0xffffff:
		idb = []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
	case id > 0xffff:
		idb = []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	case id > 0xff:
		idb = []byte{byte(id >> 8), byte(id)}
	default:
		idb = []byte{byte(id)}
	}
	// 8-byte size vint.
	size := binary.BigEndian.AppendUint64(nil, uint64(len(body)))
	size[0] = 0x01
	return append(append(idb, size...), body...)
}

// webmOf is a minimal WebM file: EBML header (DocType webm) and a Segment
// with Info (TimecodeScale 1ms, Duration in ms as float64), Tracks (one
// w×h video track) and a Cluster. durationMs < 0 omits Duration, the way a
// live MediaRecorder file does.
func webmOf(doc string, durationMs float64, w, h int) []byte {
	head := ebml(idEBML, ebml(idDocType, []byte(doc)))
	info := [][]byte{ebml(idTimecodeScale, []byte{0x0f, 0x42, 0x40})}
	if durationMs >= 0 {
		info = append(info, ebml(idDuration, binary.BigEndian.AppendUint64(nil, math.Float64bits(durationMs))))
	}
	tracks := ebml(idTracks, ebml(idTrackEntry, ebml(idVideo, ebml(idPixelWidth, []byte{byte(w >> 8), byte(w)}), ebml(idPixelHeight, []byte{byte(h >> 8), byte(h)}))))
	seg := ebml(idSegment, ebml(idInfo, info...), tracks, webmCluster())
	return append(head, seg...)
}

// webmCluster is a Cluster with a Timecode and one SimpleBlock.
func webmCluster() []byte {
	return ebml(idCluster, ebml(0xe7, []byte{0}), ebml(0xa3, []byte{0x81, 0, 0, 0x80, 'f', 'r', 'a', 'm', 'e'}))
}

// unknownSize is an 8-byte EBML size vint with every value bit set: "size
// unknown", the way a live MediaRecorder writes Segment and Cluster.
var unknownSize = []byte{0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// webmLive is a live-recording WebM: the Segment and its Cluster have an
// unknown size. clusterExtra > 0 instead gives the Cluster a known size
// that declares that many bytes more than the file holds.
func webmLive(clusterExtra int) []byte {
	head := ebml(idEBML, ebml(idDocType, []byte("webm")))
	info := ebml(idInfo, ebml(idTimecodeScale, []byte{0x0f, 0x42, 0x40}))
	tracks := ebml(idTracks, ebml(idTrackEntry, ebml(idVideo, ebml(idPixelWidth, []byte{0x02, 0x80}), ebml(idPixelHeight, []byte{0x01, 0xe0}))))
	body := append(ebml(0xe7, []byte{0}), ebml(0xa3, []byte{0x81, 0, 0, 0x80, 'f', 'r', 'a', 'm', 'e'})...)
	cluster := []byte{0x1f, 0x43, 0xb6, 0x75}
	if clusterExtra > 0 {
		size := binary.BigEndian.AppendUint64(nil, uint64(len(body)+clusterExtra))
		size[0] = 0x01
		cluster = append(cluster, size...)
	} else {
		cluster = append(cluster, unknownSize...)
	}
	cluster = append(cluster, body...)
	seg := append([]byte{0x18, 0x53, 0x80, 0x67}, unknownSize...)
	seg = append(append(append(seg, info...), tracks...), cluster...)
	return append(head, seg...)
}

// pngChunk is one PNG chunk with its CRC.
func pngChunk(typ string, data []byte) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	b = append(append(b, typ...), data...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(b[4:]))
}
