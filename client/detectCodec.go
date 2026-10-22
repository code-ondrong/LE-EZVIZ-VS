package client

const (
	CODEC_UNKNOWN = iota
	CODEC_H264
	CODEC_H265
)
const (
	TRANS_UNKNOWN = iota
	TRANS_MPEG4
	TRANS_MPEG_TS
	TRANS_MPEG_PS
	TRANS_RTP
)

func DetectCodec(buf []byte, transport int) int {
	// if transport ==
	h264 := buf[0] & 0x1f
	h265 := (buf[0] >> 1) & 0x3f
	switch h264 {
	case 7: //SPS
		return CODEC_H264
	case 8: //PPS
		return CODEC_H264
	}
	switch h265 {
	case 31: // SPS
		return CODEC_H265
	case 32: //PPS
		return CODEC_H265
	}
	return CODEC_UNKNOWN
}

// DetectRTPCodec guesses the video codec from an RTP payload's NAL header as payload type 96 is used for both.
// Only packet types that can't be mistaken for each other are used: H.264 SPS/PPS/STAP-A/FU-A land on unused
// H.265 types and H.265 VPS/SPS/PPS/AP/FU land on H.264 types 0/2/4 which cameras don't send
func DetectRTPCodec(payload []byte) int {
	if len(payload) < 2 || payload[0]&0x80 != 0 {
		return CODEC_UNKNOWN
	}
	switch (payload[0] >> 1) & 0x3f {
	case 32, 33, 34, 48, 49: // VPS, SPS, PPS, AP, FU
		return CODEC_H265
	}
	switch payload[0] & 0x1f {
	case 7, 8, 24, 28: // SPS, PPS, STAP-A, FU-A
		return CODEC_H264
	}
	return CODEC_UNKNOWN
}

func CodecName(codec int) string {
	switch codec {
	case CODEC_H264:
		return "H.264"
	case CODEC_H265:
		return "H.265"
	}
	return "unknown"
}

func DetectTransport(buf []byte) int {
	if buf[0] == 0 && buf[1] == 0 && buf[2] == 1 && buf[3] == 0xba {
		return TRANS_MPEG_PS
	}
	if buf[0] == 0x47 {
		return TRANS_MPEG_TS
	}
	rtpVersion := buf[0] >> 6
	if rtpVersion == 2 {
		return TRANS_RTP
	}
	return TRANS_UNKNOWN
}
