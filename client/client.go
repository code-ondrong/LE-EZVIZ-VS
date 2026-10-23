package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"le-ezviz-vs/api"
	"le-ezviz-vs/logging"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"go.uber.org/zap"
)

const EZLifeUrl = "ezvizlife.com"

var TerminalName = "LE-EZ"
var Client *http.Client
var log = logging.Log

// SetLogger swaps the package logger, log is captured from logging.Log at init so call this after logging.CreateLogger
func SetLogger(l *zap.Logger) {
	log = l
}

const (
	USE_API_URL = iota
	USE_DOM_URL
	USE_AUTH_URL
)

// Originally I used the mobile headers in my other EZVIZ management software, currently commented out the optional and mobile headers and just using the PC ones for this one
// I believe we might recieve different information back depending on clientType
var DefaultHeaders = http.Header{
	// "Content-Type":  []string{"application/json"},
	// "Content-Type": []string{"application/x-www-form-urlencoded"},
	"featureCode": []string{""},
	"clientType":  []string{"9"}, //3 is mobile, 9 is EZVIZ studio/PC
	// "osVersion":     []string{"15"},   // optional
	"clientVersion": []string{"2,5,1,2109068"}, //6.7.9.0507, // optional
	// "netType":       []string{"WIFI"}, // can be WIFI, LTE
	"customNo": []string{"1000001"},
	// "ssid":          []string{""},        // optional, sends the current ssid you're connected to
	"clientNo": []string{"shipin7"}, // can be web_site, apple, google, shipin7
	"appId":    []string{"ys7"},     // ys7 for yingshi and 7 is a lucky number in China, the chinese name of ezviz which translates to fluorite
	// which likely also explains the logo being different coloured fluorite crsytals.
	// "language":      []string{"en_GB"},
	// "lang":          []string{"en"},
	"User-Agent": []string{""},
}

type LE_EZVIZ_Client struct {
	ClientType    int
	ClientNo      string
	FeatureCode   string
	TerminalName  string
	Headers       http.Header
	Client        *http.Client
	LoginResponse *V3_Auth_Login_Response
	APIServerInfo *ServerInfoGetResponse
	VTDUTokens    *VTDU_TokenV2
	Email         string
	Password      string
	VerifyCode    string     // terminal bind code from SendVerifyCode, only needed when login returns 6002
	StreamOutput  string     // file the video stream is written to, "-" for stdout
	StreamType    int        // stream= in the VTM URL: 1 main stream, 2 sub stream
	Publisher     *Publisher // when set the video goes to an RTSP server instead of StreamOutput
	Region        string
	API_URL       string
	DOM_URL       string
	AUTH_URL      string
}

func NewLE_EZVIZ_Client(email, password, region, featurecode, terminalname, clientNo string, timeoutSeconds int) (*LE_EZVIZ_Client, error) {
	LEZ := &LE_EZVIZ_Client{Email: email, Password: GetMd5(password), Region: region, FeatureCode: featurecode, TerminalName: base64.StdEncoding.EncodeToString([]byte(terminalname)), ClientType: 9, ClientNo: clientNo, StreamOutput: "stream", StreamType: 1}
	if v, ok := Regions[region]; ok {
		if region != "Russia" {
			LEZ.API_URL = "https://api" + v + "." + EZLifeUrl // ex: apiieu.ezvizlife.com
			LEZ.DOM_URL = "https://" + v + "." + EZLifeUrl    // ex: ieu.ezvizlife.com
		} else {
			LEZ.API_URL = "https://api.ezvizru.com"
			LEZ.DOM_URL = "https://ezvizru.com/"
		}
	}
	LEZ.Headers = DefaultHeaders
	LEZ.Headers["featureCode"] = []string{featurecode}
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Error("Error creating cookie jar:", zap.Error(err))
		return nil, err
	}
	Client = &http.Client{
		Timeout: time.Duration(timeoutSeconds) * time.Second,
		Jar:     jar,
	}
	Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	LEZ.Client = Client
	return LEZ, nil
}

func (LEZ *LE_EZVIZ_Client) FormURLEncodedAPIRequest(method, endpoint string, urltype int, formData map[string]string) (*http.Response, error) {
	switch method {
	case "GET":
	case "POST":
	case "PATCH":
	case "PUT":
	case "DELETE":
	case "OPTIONS":
		break
	default:
		return nil, errors.New("unknown http method")
	}
	encodedFormData := EncodeURLForm(formData)
	baseURL := ""
	switch urltype {
	case USE_API_URL:
		baseURL = LEZ.API_URL
	case USE_DOM_URL:
		baseURL = LEZ.DOM_URL
	case USE_AUTH_URL:
		baseURL = LEZ.AUTH_URL
	default:
		return nil, errors.New("unknown url type")
	}
	log.Debug("Request", zap.String("URL", baseURL+endpoint), zap.String("FormData", encodedFormData))
	req, err := http.NewRequest(method, baseURL+endpoint, strings.NewReader(encodedFormData))
	if err != nil {
		log.Error("Error creating request", zap.Error(err))
		return nil, err
	}
	LEZ.Headers["Content-Type"] = []string{"application/x-www-form-urlencoded"}
	req.Header = LEZ.Headers
	resp, err := LEZ.Client.Do(req)
	if err != nil {
		log.Error("Error sending request", zap.Error(err))
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error("Status not ok", zap.Int("StatusCode", resp.StatusCode))
		return nil, errors.New("http not ok")
	}
	// bodyBytes, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	log.Error("Error reading response body", zap.Error(err))
	// 	return err
	// }
	return resp, nil
}

func (LEZ *LE_EZVIZ_Client) QueryEncodedAPIRequest(method, endpoint string, urltype int, queryParams map[string]string) (*http.Response, error) {
	switch method {
	case "GET":
	case "POST":
	case "PATCH":
	case "PUT":
	case "DELETE":
	case "OPTIONS":
		break
	default:
		return nil, errors.New("unknown http method")
	}
	encodedFormData := EncodeQuery(queryParams)
	baseURL := ""
	switch urltype {
	case USE_API_URL:
		baseURL = LEZ.API_URL
	case USE_DOM_URL:
		baseURL = LEZ.DOM_URL
	case USE_AUTH_URL:
		if LEZ.AUTH_URL == "" {
			return nil, errors.New("auth URL not initialised")
		}
		baseURL = LEZ.AUTH_URL
	default:
		return nil, errors.New("unknown url type")
	}
	log.Debug("Request", zap.String("URL", baseURL+endpoint+encodedFormData))
	req, err := http.NewRequest(method, baseURL+endpoint+encodedFormData, nil)
	if err != nil {
		log.Error("Error creating request", zap.Error(err))
		return nil, err
	}
	LEZ.Headers["Content-Type"] = []string{"application/x-www-form-urlencoded"}
	req.Header = LEZ.Headers
	resp, err := LEZ.Client.Do(req)
	if err != nil {
		log.Error("Error sending request", zap.Error(err))
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error("Status not ok", zap.Int("StatusCode", resp.StatusCode))
		return nil, errors.New("http not ok")
	}
	// bodyBytes, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	log.Error("Error reading response body", zap.Error(err))
	// 	return err
	// }
	return resp, nil
}

func init() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Error("Error creating cookie jar:", zap.Error(err))
		return
	}
	Client = &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
	}
	// Disable redirects manually
	Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
}

var ErrVerifyCodeRequired = errors.New("terminal verification code required")
var ErrVerifyCodeInvalid = errors.New("terminal verification code invalid")

// domainToURL turns a bare domain returned by the API (ex: apiisgp.ezvizlife.com) into a base URL
func domainToURL(domain string) string {
	domain = strings.TrimSuffix(domain, "/")
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return domain
	}
	return "https://" + domain
}

func (LEZ *LE_EZVIZ_Client) V3_Login() (*V3_Auth_Login_Response, error) {
	return LEZ.v3Login(1)
}

// regionRedirects is how many times we follow a 1100 (wrong region) response to the domain the API gives back
func (LEZ *LE_EZVIZ_Client) v3Login(regionRedirects int) (*V3_Auth_Login_Response, error) {
	formData := map[string]string{"account": LEZ.Email, "password": LEZ.Password, "featureCode": LEZ.FeatureCode, "cuName": base64.StdEncoding.EncodeToString([]byte(TerminalName))}
	if LEZ.VerifyCode != "" {
		// binds this featureCode to the account using the code sent by SendVerifyCode
		formData["msgType"] = "3"
		formData["bizType"] = "TERMINAL_BIND"
		formData["smsCode"] = LEZ.VerifyCode
	}
	resp, err := LEZ.FormURLEncodedAPIRequest("POST", api.V3_USER_LOGIN_V5, USE_API_URL, formData)
	if err != nil {
		log.Error("Error V3_Login", zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("Error reading response body", zap.Error(err))
		return nil, err
	}
	var data map[string]interface{}
	if err := json.NewDecoder(bytes.NewBuffer(bodyBytes)).Decode(&data); err != nil {
		log.Error("Error decoding JSON", zap.Error(err))
		return nil, err
	}
	// fmt.Println("Parsed JSON:", data)
	// fmt.Println("Response Status:", resp.Status)
	// fmt.Println("Response Header:", resp.Header)
	// fmt.Println("Response Body:", string(bodyBytes))
	// fmt.Println("Response Cookies:", resp.Cookies())
	LR := new(V3_Auth_Login_Response)
	if err := json.NewDecoder(bytes.NewBuffer(bodyBytes)).Decode(LR); err != nil {
		log.Error("Error decoding JSON", zap.Error(err))
		return nil, err
	}
	if LR.Meta.Code == nil {
		log.Error("Missing meta code in login response", zap.ByteString("Body", bodyBytes))
		return nil, errors.New("api meta code missing")
	}
	// 1100: account belongs to another region, the response tells us which API domain to use instead
	if *LR.Meta.Code == 1100 && regionRedirects > 0 && LR.LoginArea.ApiDomain != nil && *LR.LoginArea.ApiDomain != "" {
		LEZ.API_URL = domainToURL(*LR.LoginArea.ApiDomain)
		if LR.LoginArea.WebDomain != nil && *LR.LoginArea.WebDomain != "" {
			LEZ.DOM_URL = domainToURL(*LR.LoginArea.WebDomain)
		}
		log.Warn("Account is registered in a different region, retrying login on the correct server", zap.String("API_URL", LEZ.API_URL), zap.Stringp("AreaName", LR.LoginArea.AreaName))
		return LEZ.v3Login(regionRedirects - 1)
	}
	if *LR.Meta.Code != 200 {
		log.Error("Bad meta code", zap.Int("Code", *LR.Meta.Code), zap.Stringp("Message", LR.Meta.Message))
		switch *LR.Meta.Code {
		case 1100:
			log.Error("Account is registered in a different region, set the correct one with -region", zap.Stringp("ApiDomain", LR.LoginArea.ApiDomain), zap.Stringp("AreaName", LR.LoginArea.AreaName))
		case 1069:
			log.Error("Terminal Bind limit reached, remove some logged in devices in: EZVIZ App Settings -> My Profile -> Login Settings -> Terminal Management. Click and hold to delete single or tap One-Click Cleanup, this may cause a 2FA action")
		case 1226:
			log.Error("Invalid email or password")
		case 1012:
			log.Error("Verification code is invalid or expired, run again without -verifyCode to get a new one")
			return nil, ErrVerifyCodeInvalid
		case 6002:
			log.Error("This terminal is not verified yet, a verification code is required")
			return nil, ErrVerifyCodeRequired
		}
		return nil, errors.New("api meta code not ok")
	} else {
		LEZ.Headers["sessionId"] = []string{*LR.LoginSession.SessionId}
		LEZ.LoginResponse = LR
	}
	return LR, nil
}

// SendVerifyCode asks EZVIZ to send a terminal bind code to the account's email/phone, call it after V3_Login returns ErrVerifyCodeRequired
// so API_URL already points at the account's region
func (LEZ *LE_EZVIZ_Client) SendVerifyCode() error {
	resp, err := LEZ.FormURLEncodedAPIRequest("POST", api.V3_SMS_NOLOGIN_CHECKCODE, USE_API_URL, map[string]string{"from": LEZ.Email, "bizType": "TERMINAL_BIND"})
	if err != nil {
		log.Error("Error SendVerifyCode", zap.Error(err))
		return err
	}
	defer resp.Body.Close()
	var data struct {
		Meta struct {
			Code    *int    `json:"code"`
			Message *string `json:"message"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Error("Error decoding JSON", zap.Error(err))
		return err
	}
	if data.Meta.Code == nil || *data.Meta.Code != 200 {
		log.Error("Bad meta code sending verification code", zap.Intp("Code", data.Meta.Code), zap.Stringp("Message", data.Meta.Message))
		return errors.New("api meta code not ok")
	}
	return nil
}
