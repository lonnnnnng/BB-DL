package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

var icoSizes = []int{256, 128, 64, 48, 32, 16}

func main() {
	source := flag.String("source", "build/appicon.svg", "SVG source file")
	pngOutput := flag.String("png", "build/appicon.png", "1024px PNG output")
	icoOutput := flag.String("ico", "build/windows/icon.ico", "Windows ICO output")
	flag.Parse()

	icon, err := oksvg.ReadIcon(*source, oksvg.StrictErrorMode)
	if err != nil {
		fatalf("读取 SVG 失败：%v", err)
	}

	appPNG, err := renderPNG(icon, 1024)
	if err != nil {
		fatalf("生成 PNG 失败：%v", err)
	}
	if err := writeAsset(*pngOutput, appPNG); err != nil {
		fatalf("写入 PNG 失败：%v", err)
	}

	frames := make([][]byte, 0, len(icoSizes))
	for _, size := range icoSizes {
		frame, err := renderPNG(icon, size)
		if err != nil {
			fatalf("生成 %dpx Windows 图标失败：%v", size, err)
		}
		frames = append(frames, frame)
	}
	ico, err := encodeICO(icoSizes, frames)
	if err != nil {
		fatalf("生成 ICO 失败：%v", err)
	}
	if err := writeAsset(*icoOutput, ico); err != nil {
		fatalf("写入 ICO 失败：%v", err)
	}

	fmt.Printf("generated %s and %s from %s\n", *pngOutput, *icoOutput, *source)
}

func renderPNG(icon *oksvg.SvgIcon, size int) ([]byte, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))
	icon.SetTarget(0, 0, float64(size), float64(size))
	scanner := rasterx.NewScannerGV(size, size, canvas, canvas.Bounds())
	raster := rasterx.NewDasher(size, size, scanner)
	icon.Draw(raster, 1)

	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func encodeICO(sizes []int, frames [][]byte) ([]byte, error) {
	if len(sizes) == 0 || len(sizes) != len(frames) {
		return nil, fmt.Errorf("ICO 尺寸和图像数量不匹配")
	}

	var output bytes.Buffer
	if err := binary.Write(&output, binary.LittleEndian, uint16(0)); err != nil {
		return nil, err
	}
	if err := binary.Write(&output, binary.LittleEndian, uint16(1)); err != nil {
		return nil, err
	}
	if err := binary.Write(&output, binary.LittleEndian, uint16(len(frames))); err != nil {
		return nil, err
	}

	// long 2026-07-27 10:20:00：Windows 用目录表定位每个 PNG 帧，256px 的宽高必须编码为 0，否则资源编译器会把它识别成 0px 图标。
	offset := uint32(6 + 16*len(frames))
	for index, frame := range frames {
		sizeByte := byte(sizes[index])
		if sizes[index] == 256 {
			sizeByte = 0
		}
		if _, err := output.Write([]byte{sizeByte, sizeByte, 0, 0}); err != nil {
			return nil, err
		}
		if err := binary.Write(&output, binary.LittleEndian, uint16(1)); err != nil {
			return nil, err
		}
		if err := binary.Write(&output, binary.LittleEndian, uint16(32)); err != nil {
			return nil, err
		}
		if err := binary.Write(&output, binary.LittleEndian, uint32(len(frame))); err != nil {
			return nil, err
		}
		if err := binary.Write(&output, binary.LittleEndian, offset); err != nil {
			return nil, err
		}
		offset += uint32(len(frame))
	}
	for _, frame := range frames {
		if _, err := output.Write(frame); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func writeAsset(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
