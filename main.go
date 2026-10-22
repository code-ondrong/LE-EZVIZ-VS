package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	client "le-ezviz-vs/client"
	logging "le-ezviz-vs/logging"
	"os"

	"go.uber.org/zap"
)

// email, password and verifyCode fall back to the environment after parsing so their values never show up in -help
var email = flag.String("email", "", "EZVIZ user e-mail (env EZVIZ_EMAIL)")
var password = flag.String("password", "", "EZVIZ user password (env EZVIZ_PASSWORD)")
var verifyCode = flag.String("verifyCode", "", "Verification code EZVIZ sends to your email/phone the first time this terminal logs in (env EZVIZ_VERIFY_CODE)")
var region = flag.String("region", envOr("EZVIZ_REGION", "Asia"), "Manually set the region you are in: Europe, Africa, Asia, India, Oceania, NorthAmerica, Russia, SouthAmerica (env EZVIZ_REGION)")
var terminalName = flag.String("terminalName", "LE-EZ", "Optional: Set the name of what your device is called when viewing registered devices (terminals)")
var preservefc = flag.Bool("preserveFeatureCode", true, "Preserve the featurecode, it is essentially a random ID to identify the terminal")
var deviceSerial = flag.String("deviceSerial", envOr("EZVIZ_DEVICE_SERIAL", ""), "The device serial you want to connect to (env EZVIZ_DEVICE_SERIAL)")
var preserveSession = flag.Bool("preserveSession", false, "Save the login session to session.json and reuse it while valid, lets several processes share one login")
var output = flag.String("output", "stream", "File the video stream is written to, - writes it to stdout (logs then go to stderr) to pipe into ffmpeg")
var logLevel = flag.String("logLevel", envOr("LOG_LEVEL", "debug"), "Log level: debug, info, warn, error (env LOG_LEVEL)")
var stdout = flag.Bool("stdout", true, "Print log to the terminal")
var logFile = flag.Bool("logFile", true, "Print log to lez.log")
var log *zap.Logger

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// login logs in, with a session file it reuses the saved session and logs in again if the API rejects it
func login(LEZ *client.LE_EZVIZ_Client, sessionFile string) error {
	if sessionFile == "" {
		if _, err := LEZ.V3_Login(); err != nil {
			return err
		}
		_, err := LEZ.GetServerInfo()
		return err
	}
	cached, err := LEZ.LoginCached(sessionFile)
	if err != nil {
		return err
	}
	if _, err = LEZ.GetServerInfo(); err != nil && cached {
		log.Warn("Saved session rejected, logging in again")
		LEZ.ForgetSession(sessionFile)
		if _, err = LEZ.LoginCached(sessionFile); err != nil {
			return err
		}
		_, err = LEZ.GetServerInfo()
	}
	return err
}

func main() {
	flag.Parse()
	if *email == "" {
		*email = os.Getenv("EZVIZ_EMAIL")
	}
	if *password == "" {
		*password = os.Getenv("EZVIZ_PASSWORD")
	}
	if *verifyCode == "" {
		*verifyCode = os.Getenv("EZVIZ_VERIFY_CODE")
	}
	if *email == "" {
		panic("email empty")
	}
	if *password == "" {
		panic("password empty")
	}
	console := ""
	if *stdout {
		console = "stdout"
		if *output == "-" {
			console = "stderr"
		}
	}
	if err := logging.CreateLogger(*logFile, console, *logLevel); err != nil {
		panic(err)
	}
	client.CurrentRegion = *region
	client.TerminalName = *terminalName
	log = logging.Log
	if *deviceSerial != "" {
		log = log.With(zap.String("Device", *deviceSerial))
	}
	client.SetLogger(log)
	if _, ok := client.Regions[*region]; !ok {
		log.Error("Invalid region", zap.String("Valid Values", "Europe|Africa|Asia|India|Oceania|NorthAmerica|Russia|SouthAmerica"))
		return
	}
	LEZ, err := client.NewLE_EZVIZ_Client(*email, *password, *region, "00000000000000000000000000000000", *terminalName, "shipin7", 15)
	if err != nil {
		panic(err)
	}
	if *preservefc {
		LEZ.LoadFeatureCode("featurecode")
	} else {
		LEZ.SetFeatureCode(hex.EncodeToString(client.GenerateFeatureCode()))
	}
	LEZ.VerifyCode = *verifyCode
	LEZ.StreamOutput = *output
	sessionFile := ""
	if *preserveSession {
		sessionFile = "session.json"
	}
	if err = login(LEZ, sessionFile); err != nil {
		if errors.Is(err, client.ErrVerifyCodeRequired) && *verifyCode == "" {
			if err = LEZ.SendVerifyCode(); err != nil {
				panic(err)
			}
			fmt.Fprintln(os.Stderr, "A verification code has been sent to your EZVIZ account email/phone, run again with -verifyCode=\"CODE\"")
			return
		}
		panic(err)
	}

	PageList, err := LEZ.GetPageList()
	if err != nil {
		panic(err)
	}
	_, err = LEZ.GetVTDUv2Token()
	if err != nil {
		panic(err)
	}
	for _, v := range *PageList.DeviceInfos {
		log.Info(v.Name, zap.String("Serial", v.DeviceSerial))
	}
	// fmt.Println(PageList.VTM)
	fmt.Fprintln(os.Stderr, "!!!WARNING: This library is in beta, only use for development/testing until it is stable, things will change as development continues!!!")
	fmt.Fprintln(os.Stderr, "!!!Encryption is not yet available including E2EE with stream servers, your streams will be unencrypted until encryption is implemented!!!")
	if *deviceSerial != "" {
		var RI client.Resource
		var DI client.DeviceInfos
		for _, v := range *PageList.ResourceInfos {
			if *deviceSerial == v.DeviceSerial {
				RI = v
			}
		}
		if v, ok := PageList.VTM[RI.ResourceID]; ok {
			VS, err := LEZ.ConnectVTM(v.ExternalIP, v.Port, RI, DI, "", v.PublicKey.Key)
			if err != nil {
				log.Error("Error connecting VTM", zap.Error(err))
				return
			}
			Tokens := *LEZ.VTDUTokens.Tokens
			URL := LEZ.BuildVtmUrl(VS.VTMIP, VS.VTMPort, RI.DeviceSerial, RI.StreamBizUrl, Tokens[0], DI.ChannelNumber, LEZ.ClientType)
			RStreamInfoRsp, err := LEZ.StartVTMStream(VS, URL)
			if err != nil {
				panic(err)
			}

			IP, Port, _, _, err := LEZ.ParseVtmUrl(*RStreamInfoRsp.Streamurl)
			if err != nil {
				panic(err)
			}
			VTDUStream, err := LEZ.ConnectVTDU(IP, Port, *RStreamInfoRsp.Vtmstreamkey, v.PublicKey.Key)
			if err != nil {
				panic(err)
			}
			err = LEZ.StartVTDUStream(VTDUStream, URL)
			if err != nil {
				panic(err)
			}
		} else {
			log.Error("Device serial not found on this account or it has no VTM server")
			os.Exit(1)
		}
	}

}
