package client

import (
	"io"
	"os"
	"os/exec"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	"go.uber.org/zap"
)

// Publisher sends a camera's video to an RTSP server such as MediaMTX. RTP video is forwarded as it arrives so
// the camera's own timestamps are kept and nothing has to probe the stream first, MPEG-PS is handed to ffmpeg.
type Publisher struct {
	URL    string
	codec  int
	rtsp   *gortsplib.Client
	desc   *description.Session
	ffmpeg *exec.Cmd
	stdin  io.WriteCloser
}

func NewPublisher(url string) *Publisher {
	return &Publisher{URL: url}
}

// WriteRTP publishes one RTP packet if it carries video (payload type 96), the other payloads are audio and
// EZVIZ private data
func (p *Publisher) WriteRTP(buf []byte) error {
	var pkt rtp.Packet
	if err := pkt.Unmarshal(buf); err != nil {
		return err
	}
	if pkt.PayloadType != 96 {
		return nil
	}
	if p.rtsp == nil {
		// the SPS usually comes first and tells H.264 and H.265 apart, packets before it are dropped
		if p.codec = DetectRTPCodec(pkt.Payload); p.codec == CODEC_UNKNOWN {
			return nil
		}
		if err := p.startRTSP(); err != nil {
			return err
		}
	}
	return p.rtsp.WritePacketRTP(p.desc.Medias[0], &pkt)
}

func (p *Publisher) startRTSP() error {
	var forma format.Format = &format.H264{PayloadTyp: 96, PacketizationMode: 1}
	if p.codec == CODEC_H265 {
		forma = &format.H265{PayloadTyp: 96}
	}
	p.desc = &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{forma},
	}}}
	protocol := gortsplib.ProtocolTCP
	client := &gortsplib.Client{Protocol: &protocol}
	if err := client.StartRecording(p.URL, p.desc); err != nil {
		log.Error("Error publishing to RTSP server", zap.String("URL", p.URL), zap.Error(err))
		return err
	}
	p.rtsp = client
	log.Info("Publishing RTP video", zap.String("URL", p.URL), zap.String("Codec", CodecName(p.codec)))
	return nil
}

// WritePS hands MPEG-PS to ffmpeg, which publishes the video. The PS timestamps of the tested camera never let
// MediaMTX finish an HLS segment, so the arrival time is used instead
func (p *Publisher) WritePS(buf []byte) error {
	if p.ffmpeg == nil {
		cmd := exec.Command("ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "warning",
			"-use_wallclock_as_timestamps", "1", "-probesize", "1M", "-analyzeduration", "1M", "-f", "mpeg", "-i", "pipe:0",
			"-map", "0:v:0", "-c", "copy", "-f", "rtsp", "-rtsp_transport", "tcp", p.URL)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			log.Error("Error starting ffmpeg", zap.Error(err))
			return err
		}
		p.ffmpeg, p.stdin = cmd, stdin
		log.Info("Publishing MPEG-PS video through ffmpeg", zap.String("URL", p.URL))
	}
	_, err := p.stdin.Write(buf)
	return err
}

func (p *Publisher) Close() {
	if p.rtsp != nil {
		p.rtsp.Close()
	}
	if p.ffmpeg != nil {
		p.stdin.Close()
		p.ffmpeg.Wait()
	}
}
