# LE-EZVIZ Video Stream

LE-EZVIZ-VS, this is a piece of my wider project of creating a fully fledged program to control and connect to EZVIZ cameras in my own implementation. I have many modules as it makes testing easier and more will be released when ready. I am releasing this to hopefully spur on more development and get more help and hands on with the streaming implementation as not much is out there.

Modern EZVIZ cameras have locked down their management ports which is disappointing. I want to record these streams or use them for home automation and using AI to detect faces and objects as I am sure many others want to do as well. It would not only be cheaper and more versatile for them and us to have the option of re-enabling those ports and having local control, it would save them costs of them having to stream and re-encode video streams and would also give us higher bitrate streams rather than cutting it down for internet transmission but I digress.

I have developed an example of connecting to EZVIZ Video Management System. I'll delve into a little on how it works, during the login flow it gives a VTM address you connect to for video streams, VTM I believe stands for Video Transmission Manangement. VTM as I observe is a load balancer that then points you to a VTDU (I believe this means Video Transmission Data/Distribution Unit) that is the server that streams the video feed.

## Compatability (WIP)

Key | ✅ = supported | ❌ = Not supported (WIP) | ❓= Unknown/May work (If you test create an issue)

| Models  | Codec | Support Video | Support Audio |
| ------- | ----- | ------------- | ------------- |
| DB1     | ❓    | ❓            | ❓            |
| DB2     | ❓    | ❓            | ❓            |
| HP2     | PS    | ✅            | ✅            |
| HP3     | RTP   | ✅            | ❌            |
| CB3-R1  | PS    | ✅            | ✅            |
| CB3-R2  | ❓    | ❓            | ❓            |
| CB2     | PS    | ✅            | ✅            |
| CB1     | PS    | ✅            | ✅            |
| E/CB5   | RTP   | ✅            | ❌            |
| CP1     | PS    | ✅            | ✅            |
| CP1 pro | ❓    | ❓            | ❓            |
| CP2     | PS    | ✅            | ✅            |
| C6N     | PS    | ✅            | ✅            |
| CB8     | RTP   | ✅            | ❌            |
| HB8     | RTP   | ✅            | ❌            |
| HB3     | ❓    | ❓            | ❓            |
| BC1     | ❓    | ❓            | ❓            |
| H9c     | RTP   | ✅            | ❌            |

## Limitations

Currently encryption is unsupported, I have a decent idea on how it works. I do have bits of information missing. My expertise in using x64dbg is limited but I am always looking into it.

RTP audio is WIP, re-streaming to other formats is on the to-do list, network stability can affect the decoding of the TCP stream no live demuxer/retransmission/FEC is implemented

As seen with the VTM packet format the channel depicts whether it is an encrypted message or stream (0x00 for unencrypted message and 0x01 for unecrypted stream, 0x0a for encrypted message and 0x0b for encrypted stream), most of the time it is encrypted but some do not use it which is mainly older cameras. There are effectively 2 layers of encryption as you may guess from the Image/Video Encryption option. There is E2EE with VTM/VTDU which is still yet to be implemented and then the stream is encrypted with the password you set. The E2EE is ECDHE with Prime-256 curve, the stream I believe is AES-128 I am unsure which cipher mode it is but we'll cross the encryption bridge when we have a good foundation.

As I am only one person looking into this and not seeing much else online, information is quite scarce and with a limited amount of tools and knowledge I cannot provide a 100% implementation yet. That is why I am releasing this in it's current state to share my knowledge and induce collaboration and work on this so we can reimplement video streaming capabilities that we should have access to.

## What currently works

Currently as of 2026-01-04, MPEG-PS streaming works and so does H.265 RTP streams with no encryption enabled.

## If you want to help

If you want to help contribute, provide information or reimplement in another language it would be highly appreciated, feel free to join my [discord](https://discord.gg/3k5eBGsBkN) and if you need any pointers or help on where to look that's the best place to go to so we can give updates and share information. If you are creating a reimplementation let me know on github or discord and I'll create a list and add it to the repo or if you want to become a collaborator and create a branch. Remember to not include any sensitive information such as your keys, serials, tokens, password and any other kind of account information.

## To compile and run

Download [golang](https://go.dev/dl/) and install, open terminal and run `go build`. To use and run `./le-ezviz-vs -email="your@email.com" -password="password"` with no deviceSerial provided it will list all your devices, to have it begin the stream which for now doesn't do much but is a PoC for now add -deviceSerial like so `./le-ezviz-vs -email="your@email.com" -password="password" -deviceSerial="BC0123456"`

This will begin the stream and start spitting out the first 64 bytes of packets and their sequence. The stream may end after a while as I haven't setup the keepalive timer yet, the code isn't perfect and things will change but it does illustrate the things I have researched.

## Docker / Coolify

The Docker image bundles le-ezviz-vs with [MediaMTX](https://github.com/bluenviron/mediamtx), ffmpeg and `cctvd` so the cameras can be watched in a browser over HLS.

- A camera is only streamed while someone is watching it (MediaMTX `runOnDemand`, stopped 30 seconds after the last viewer leaves).
- RTP cameras are published straight to MediaMTX with the camera's own timestamps (`-publish`), MPEG-PS cameras go through ffmpeg. Together with Low-Latency HLS a cold camera plays about 5 seconds after it is opened.
- All camera processes share one EZVIZ login saved in `/data/session.json`.
- `cctvd` serves the public port. It passes HLS through to MediaMTX and adds `/status.json` (which cameras are online, from EZVIZ, with their latest thumbnail) and `/thumb/<name>.jpg` (480px wide, taken every `THUMB_LIVE_INTERVAL` while a camera is watched, otherwise every `THUMB_IDLE_INTERVAL` by starting its stream briefly).

| Variable | Default | Description |
| --- | --- | --- |
| `EZVIZ_EMAIL` | required | EZVIZ account e-mail |
| `EZVIZ_PASSWORD` | required | EZVIZ account password |
| `CAMERAS` | required | `name:SERIAL` pairs separated by commas, ex: `front-door:BC0123456,garden:BC0654321`. The name is the stream path |
| `EZVIZ_REGION` | `Asia` | Region of the account, a wrong region is corrected automatically on login |
| `ALWAYS_ON` | empty | Camera names to stream (and record) all the time instead of only while watched |
| `SUBSTREAM` | `true` | Also serve each camera's sub stream (lower resolution, ex: 768x432) at `<name>-sd` |
| `HLS_VARIANT` | `lowLatency` | `lowLatency` or `fmp4`. fMP4 starts ~4 seconds slower but Safari's native player never trips over cameras with uneven frame timing |
| `THUMB_LIVE_INTERVAL` | `30s` | Thumbnail refresh while a camera is watched |
| `THUMB_IDLE_INTERVAL` | `15m` | Thumbnail refresh while nobody watches, each one starts the camera for a few seconds |
| `RECORD` | `true` | Record streams while they are live |
| `RECORD_DELETE_AFTER` | `1d` | How long recordings are kept |
| `LOG_LEVEL` | `info` | `debug` logs every packet, only use it for troubleshooting |
| `TZ` | `Asia/Makassar` | Time zone used for recording file names |

Streams are public and read-only at `https://<domain>/<name>/index.m3u8` (sub stream: `https://<domain>/<name>-sd/index.m3u8`), MediaMTX also serves a player page at `https://<domain>/<name>/`. Only processes inside the container can publish. In Coolify use the Docker Compose build pack and set the domain of the `cctv` service with port 8080, ex: `https://cctv.example.com:8080`. Locally put the variables in a `.env` file and run `docker compose up`.

The same variables work outside Docker: `EZVIZ_EMAIL`, `EZVIZ_PASSWORD`, `EZVIZ_REGION` and `LOG_LEVEL` are used when the matching flag is not given.

## My methods of reverse engineering

I have used a multitude of programs to help aid my efforts as listed below, research into different types of protocols and video formats also helped. What helped the most was packet analysis and finding bits and pieces of information and patterns along with looking at headers, structs and enums in ghidra.

* Ghidra
* x64dbg
* IEChooser
* Wireshark
* Charles Proxy
* MITMProxy
* Coffee, lots and lots of it
* Sleepless nights
* Shower thoughts
* Proxifier
* IDA
* Notepad++ and HxD
* MPEG-PS analyser
