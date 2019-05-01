package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
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
	*wavreader.Wav
	q16 *locklessq.Qint16
	q32 *locklessq.Qfloat32
}

func (p *play) Write(buff []byte) (int, error) {
	if p.q16 != nil {
		return p.write16(buff)
	}
	return p.write32(buff)
}

func (p *play) write16(buff []byte) (int, error) {
	for i := 0; i < len(buff); i = i + 2 {
		p.q16.Insert(conv.BytesToint16(buff, i))
	}
	return len(buff), nil
}

func (p *play) write32(buff []byte) (int, error) {
	for i := 0; i < len(buff); i = i + 4 {
		p.q32.Insert(conv.BytesToFloat32(buff, i))
	}
	return len(buff), nil
}

func (p *play) CallBack(inputBuffer, outputBuffer unsafe.Pointer, frames uint64) {
	if p.q16 != nil {
		p.cb16(inputBuffer, outputBuffer, frames)
		return
	}
	p.cb32(inputBuffer, outputBuffer, frames)
}

func (p *play) cb16(inputBuffer, outputBuffer unsafe.Pointer, frames uint64) {
	ob := (*[1024]int16)(outputBuffer)
	for i := 0; i < len(ob); i++ {
		val, _ := p.q16.Pop()
		(*ob)[i] = val
	}
}

func (p *play) cb32(inputBuffer, outputBuffer unsafe.Pointer, frames uint64) {
	ob := (*[1024]float32)(outputBuffer)
	for i := 0; i < len(ob); i++ {
		val, _ := p.q32.Pop()
		(*ob)[i] = val
	}
}

func main() {

	action := flag.String("action", "recnplay", "recnplay - records and plays\nrecnsave - records and saves the buffer as wav\nplay - plays a file")
	file := flag.String("file", "/Users/elerer/samples/temp.wav", "path to save file as wav")
	sec := flag.Uint64("sec", 4, "seconds to record")
	uisf := flag.Uint64("sf", 8, "1 - 32 bit float\n8 - 16 bit")

	var (
		si        *record
		player    *play
		in        pa.PaStreamParameters
		desiredSR uint64
		sf        pa.SampleFormat
		channels  = 1
	)

	flag.Parse()

	sf = pa.SampleFormat(*uisf)

	if strings.Contains(*action, "rec") {
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

		println("selected  ", devnum, " sample format ", sf)

		in = pa.PaStreamParameters{DeviceNum: devnum, ChannelCount: channels, Sampleformat: sf}
		desiredSR = 44100
		err = pa.IsformatSupported(&in, nil, desiredSR)

		if err != nil {
			println("ERROR ", err.Error())
			return
		}

		fmt.Println(in, " supports ", desiredSR)

		si = &record{q: locklessq.NewQint16(int32(desiredSR * (*sec)))}

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

	} else if *action == "play" {
		err := pa.Initialize()
		if err != nil {
			println("ERROR ", err.Error())
			return
		}
		defer pa.Terminate()

		wr, err := wavreader.New(*file)
		if err != nil {
			log.Fatal(err)
		}
		wr.String()
		defer wr.Close()
		player = &play{Wav: wr}

		bps := player.BitsPerSample()

		bytesPerSample := uint32(bps / 8)

		buffsize := int32(wr.DataBytesCount() / bytesPerSample)

		println("Allocating ", buffsize, " elems")

		if bps == 16 {
			player.q16 = locklessq.NewQint16(buffsize)
			sf = pa.Int16
			io.Copy(player, player)
		} else if bps == 32 {
			player.q32 = locklessq.NewQfloat32(buffsize)
			sf = pa.Float32
			io.Copy(player, player)
		} else {
			log.Fatal("Unsupported bps ", bps)
		}

		pa.ListDevices()
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Select device num for play: ")
		text, _ := reader.ReadString('\n')

		devnum, err := strconv.Atoi(strings.TrimSuffix(text, "\n"))
		if err != nil {
			println(err)
			return
		}

		println("selected  ", devnum, " sample format ", sf)
		out := pa.PaStreamParameters{DeviceNum: devnum, ChannelCount: int(player.NumChannels()), Sampleformat: sf}

		pa.CbStream = player
		s, err := pa.OpenStream(nil, &out, in.Sampleformat, uint64(player.SampleRate()), 1024)
		if err != nil {
			println("ERROR ", err.Error())
			return
		}

		s.Start()
		println("playing...")
		time.Sleep(4 * time.Second)
		s.Stop()
		s.Close()

		return

	}

	//play recording
	if strings.Contains(*action, "play") {
		pa.ListDevices()
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Select device num for play: ")
		text, _ := reader.ReadString('\n')
		devnum, err := strconv.Atoi(strings.TrimSuffix(text, "\n"))
		if err != nil {
			println(err)
			return
		}

		println("selected  ", devnum)
		out := pa.PaStreamParameters{DeviceNum: devnum, ChannelCount: channels, Sampleformat: sf}

		player = &play{q16: si.q}

		pa.CbStream = player
		s, err := pa.OpenStream(nil, &out, in.Sampleformat, desiredSR, 1024)
		if err != nil {
			println("ERROR ", err.Error())
			return
		}

		s.Start()
		println("playing...")
		time.Sleep(4 * time.Second)
		s.Stop()
		s.Close()

	} else if *action == "recnsave" {
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
