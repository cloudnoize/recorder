package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/cloudnoize/conv"
	"github.com/cloudnoize/elport"
	locklessq "github.com/cloudnoize/locklessQ"
	"github.com/cloudnoize/wavreader"
)

type record struct {
	q *locklessq.Qint16
}

func (r *record) CallBack(inputBuffer, outputBuffer unsafe.Pointer, frames uint64) {
	ib := (*[1024]int16)(inputBuffer)
	for i := 0; i < len(ib); i++ {
		r.q.Insert((*ib)[i])
	}
}

type play struct {
	q *locklessq.Qint16
}

func (p *play) CallBack(inputBuffer, outputBuffer unsafe.Pointer, frames uint64) {
	ob := (*[1024]int16)(outputBuffer)
	for i := 0; i < len(ob); i++ {
		val, _ := p.q.Pop()
		(*ob)[i] = val
	}
}

func main() {

	action := flag.String("action", "play", "play - plats the recorded buffer\nsave - saves the buffer as wav")
	file := flag.String("file", "/Users/elerer/samples/temp.wav", "path to save file as wav")
	sec := flag.Uint64("sec", 4, "seconds to record")

	flag.Parse()

	err := pa.Initialize()
	if err != nil {
		println("ERROR ", err.Error())
		return
	}
	defer pa.Terminate()

	pa.ListDevices()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Select device num for record: ")
	text, _ := reader.ReadString('\n')
	devnum, err := strconv.Atoi(strings.TrimSuffix(text, "\n"))
	if err != nil {
		fmt.Println(err)
		return
	}

	println("selected  ", devnum)

	in := pa.PaStreamParameters{DeviceNum: devnum, ChannelCount: 1, Sampleformat: pa.Int16}
	desiredSR := uint64(44100)
	err = pa.IsformatSupported(&in, nil, desiredSR)

	if err != nil {
		println("ERROR ", err.Error())
		return
	}

	fmt.Println(in, " supports ", desiredSR)

	si := &record{q: locklessq.NewQint16(int32(desiredSR * (*sec)))}

	pa.CbStream = si

	//Open stream
	s, err := pa.OpenStream(&in, nil, in.Sampleformat, desiredSR, 1024)
	if err != nil {
		println("ERROR ", err.Error())
		return
	}

	s.Start()
	println("recording...")
	time.Sleep(time.Duration(*sec) * time.Second)
	s.Stop()
	s.Close()

	//play
	if *action == "play" {
		pa.ListDevices()
		reader = bufio.NewReader(os.Stdin)
		fmt.Print("Select device num for play: ")
		text, _ = reader.ReadString('\n')
		devnum, err = strconv.Atoi(strings.TrimSuffix(text, "\n"))
		if err != nil {
			println(err)
			return
		}

		println("selected  ", devnum)
		out := pa.PaStreamParameters{DeviceNum: devnum, ChannelCount: 1, Sampleformat: pa.Int16}

		sp := &play{q: si.q}

		pa.CbStream = sp
		s, err = pa.OpenStream(nil, &out, in.Sampleformat, desiredSR, 1024)
		if err != nil {
			println("ERROR ", err.Error())
			return
		}

		s.Start()
		println("playing...")
		time.Sleep(4 * time.Second)
		s.Stop()
		s.Close()

	} else if *action == "save" {
		var wh wavreader.WavHHeader
		var buff [44]byte
		//CHunkID
		copy(buff[:], []byte("RIFF"))
		//ChunkSize
		audioBytes := len(si.q.Q) * 2        //16bits e.g. 2 bytes
		chunksize := uint32(38 + audioBytes) //38 for rest of header
		conv.UInt32ToBytes(chunksize, buff[:], 4)
		//Format
		copy(buff[8:], []byte("WAVE"))
		//Subchunk1ID
		copy(buff[12:], []byte("fmt "))
		//Subchunk1Size
		conv.UInt32ToBytes(16, buff[:], 16)
		//Audioformat
		buff[20] = 1
		//NumChannels
		buff[22] = 1
		//Samplerate
		conv.UInt32ToBytes(44100, buff[:], 24)
		//Byterate
		conv.UInt32ToBytes(44100*1*16/8, buff[:], 28)
		//BlockAlign
		ba := byte(1 * 16 / 8)
		buff[32] = ba
		//BitsPerSample
		buff[34] = 16
		//Subchunk2ID
		copy(buff[36:], []byte("data"))
		//Subchunk2Size
		conv.UInt32ToBytes(uint32(audioBytes), buff[:], 40)

		wh.Hdr = append(wh.Hdr, buff[:]...)
		wh.String()

		// If the file doesn't exist, create it, or append to the file
		f, err := os.OpenFile(*file, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := f.Write(buff[:]); err != nil {
			log.Fatal(err)
		}
		//Write data
		bytebuff := make([]byte, audioBytes, audioBytes)
		println(audioBytes, len(si.q.Q))
		for i, v := range si.q.Q {
			conv.Int16ToBytes(v, bytebuff, i*2)
		}
		if _, err := f.Write(bytebuff); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}

}
