//go:build windows

package clipboard

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestDIBToPNGHandlesBottomUpAndZeroAlpha(t *testing.T) {
	const width, height = 2, 2
	const headerSize = 40
	stride := width * 4
	dib := make([]byte, headerSize+stride*height)

	binary.LittleEndian.PutUint32(dib[0:], headerSize)
	binary.LittleEndian.PutUint32(dib[4:], width)
	binary.LittleEndian.PutUint32(dib[8:], height) // 正数：bottom-up
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	binary.LittleEndian.PutUint32(dib[16:], 0) // BI_RGB
	binary.LittleEndian.PutUint32(dib[20:], uint32(stride*height))

	// 底行（内存中先出现）：蓝、绿；顶行：红、白。alpha 全为 0，应回退为不透明。
	bottom := dib[headerSize:]
	bottom[0], bottom[1], bottom[2], bottom[3] = 255, 0, 0, 0
	bottom[4], bottom[5], bottom[6], bottom[7] = 0, 255, 0, 0
	top := dib[headerSize+stride:]
	top[0], top[1], top[2], top[3] = 0, 0, 255, 0
	top[4], top[5], top[6], top[7] = 255, 255, 255, 0

	out, err := dibToPNG(dib)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	check := func(x, y int, wr, wg, wb uint32) {
		r, g, b, a := img.At(x, y).RGBA()
		if a != 0xffff {
			t.Fatalf("pixel (%d,%d) alpha = %d, want opaque", x, y, a)
		}
		if r != wr || g != wg || b != wb {
			t.Fatalf("pixel (%d,%d) = (%d,%d,%d), want (%d,%d,%d)", x, y, r, g, b, wr, wg, wb)
		}
	}
	check(0, 0, 0xffff, 0, 0)           // 左上红
	check(1, 0, 0xffff, 0xffff, 0xffff) // 右上白
	check(0, 1, 0, 0, 0xffff)           // 左下蓝
	check(1, 1, 0, 0xffff, 0)           // 右下绿
}
