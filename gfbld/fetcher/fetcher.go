package fetcher

import (
	"GFBLD/gfbld/database"
	"fmt"
	x2j "github.com/basgys/goxml2json"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"
	"github.com/valyala/fastjson"
	"gorm.io/gorm"
	"strings"
	"time"
)

const (
	ua  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.84 Safari/537.36"
	ct  = "application/x-www-form-urlencoded"
	sfs = "same-origin"
	ae  = "gzip, deflate, br"
)

type Fetcher struct {
	id      int
	vlDocId int
	viDocId int
	req     *fasthttp.Request
	res     *fasthttp.Response
	client  *fasthttp.Client
}

func NewFetcher(proxy string, id int, vlDocId, viDocId int) *Fetcher {
	logrus.Info("create fetcher")
	return &Fetcher{
		id:      id,
		vlDocId: vlDocId,
		viDocId: viDocId,
		req:     &fasthttp.Request{},
		res:     &fasthttp.Response{},
		client: &fasthttp.Client{
			ReadBufferSize: 8192,
			Dial:           fasthttpproxy.FasthttpHTTPDialer(proxy),
		},
	}
}

func (f *Fetcher) FetchLiveList() []*database.LiveRecord {
	logrus.Info("fetch live videos list")
	return f.getLiveList(nil, "")
}

func (f *Fetcher) FetchLiveInfo(ll []*database.LiveRecord, db *gorm.DB) {
	logrus.Info("fetch live videos' info")
	f.getLiveInfo(ll, db)
}

func (f *Fetcher) getLiveList(ll []*database.LiveRecord, cursor string) []*database.LiveRecord {
	if ll == nil {
		ll = make([]*database.LiveRecord, 0)
	}

	f.req.SetRequestURI("https://www.facebook.com/api/graphql/")
	f.req.Header.SetMethod(fasthttp.MethodPost)
	f.req.Header.SetUserAgent(ua)
	f.req.Header.SetContentType(ct)
	f.req.Header.Set("Sec-Fetch-Site", sfs)
	f.req.Header.Set("Accept-Encoding", ae)
	f.req.SetBodyString(fmt.Sprintf(`variables={"cursor":"%s","id":"%d"}&doc_id=%d`, cursor, f.id, f.vlDocId))

	if err := f.client.Do(f.req, f.res); err != nil {
		logrus.WithError(err).Error("failed to get video list")
		return nil
	}

	body := readBody(f.res)
	jd, err := fastjson.ParseBytes(body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"body": string(body),
			"err":  err,
		}).Error("failed to parse json body")
		return nil
	}

	for _, v := range jd.GetArray("data", "node", "all_lives", "edges") {
		temp := &database.LiveRecord{
			LiveId:     string(v.GetStringBytes("node", "id")),
			CreateTime: time.Now().Format("2006-01-02 15:04:05"),
			UploadTime: time.Unix(v.GetInt64("node", "created_time"), 0).Format("2006-01-02 15.04.05"),
			Name:       string(v.GetStringBytes("node", "savable_title", "text")),
		}

		ll = append(ll, temp)
	}

	if jd.GetBool("data", "node", "all_lives", "page_info", "has_next_page") {
		cursor = string(jd.GetStringBytes("data", "node", "all_lives", "page_info", "end_cursor"))
		ll = f.getLiveList(ll, cursor)
	}
	return ll
}

func (f *Fetcher) getLiveInfo(ll []*database.LiveRecord, db *gorm.DB) {
	for _, l := range ll {
		f.req.SetRequestURI("https://www.facebook.com/api/graphql/")
		f.req.Header.SetMethod(fasthttp.MethodPost)
		f.req.Header.SetUserAgent(ua)
		f.req.Header.SetContentType(ct)
		f.req.Header.Set("Sec-Fetch-Site", sfs)
		f.req.Header.Set("Accept-Encoding", ae)
		f.req.SetBodyString(fmt.Sprintf(`variables={"videoID":"%s"}&doc_id=%d`, l.LiveId, f.viDocId))

		if err := f.client.Do(f.req, f.res); err != nil {
			logrus.WithError(err).Error("failed to get video list")
			return
		}

		body := readBody(f.res)
		bodyString := strings.Split(string(body), `{"label"`)[0]
		jd, err := fastjson.Parse(bodyString)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"body": string(body),
				"err":  err,
			}).Error("failed to parse json body")
			return
		}
		xmlString := string(jd.GetStringBytes("data", "video", "dash_manifest"))
		if len(xmlString) == 0 {
			logrus.WithField("raw", string(body)).Error("empty response data")
			continue
		}

		xmlReader := strings.NewReader(xmlString)
		xjBuffer, err := x2j.Convert(xmlReader)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"raw": xmlString,
				"err": err,
			}).Error("failed to parse xml to json")
			continue
		}

		xjBytes := xjBuffer.Bytes()

		xjd, err := fastjson.ParseBytes(xjBytes)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"raw": string(xjBytes),
				"err": err,
			}).Error("failed to parse xml-to-json json bytes")
			continue
		}

		vu := ""
		au := ""

		for _, as := range xjd.GetArray("MPD", "Period", "AdaptationSet") {
			if len(string(as.GetStringBytes("-maxWidth"))) != 0 {
				mw := string(as.GetStringBytes("-maxWidth"))
				for _, res := range as.GetArray("Representation") {
					if string(res.GetStringBytes("-width")) == mw {
						vu = string(res.GetStringBytes("BaseURL"))
					}
				}
			}
			if len(string(as.GetStringBytes("Representation", "-audioSamplingRate"))) != 0 {
				au = string(as.GetStringBytes("Representation", "BaseURL"))
			}
		}

		videoRecord := &database.Video{}
		audioRecord := &database.Audio{}
		if err := db.Where("belongs_to = ?", l.LiveId).First(&videoRecord).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				db.Save(&database.Video{
					BelongsTo:  l.LiveId,
					CreateTime: time.Now().Format("2006-01-02 15:04:05"),
					Name:       l.Name,
					Url:        vu,
				})
			} else {
				logrus.WithError(err).Error("failed to retrieve video record")
			}
		} else {
			videoRecord.Url = vu
			db.Save(&videoRecord)
		}
		if err := db.Where("belongs_to = ?", l.LiveId).First(&audioRecord).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				db.Save(&database.Audio{
					BelongsTo:  l.LiveId,
					CreateTime: time.Now().Format("2006-01-02 15:04:05"),
					Name:       l.Name,
					Url:        au,
				})
			} else {
				logrus.WithError(err).Error("failed to retrieve video record")
			}
		} else {
			audioRecord.Url = au
			db.Save(&audioRecord)
		}
	}

	return
}

func readBody(res *fasthttp.Response) []byte {
	encoding := res.Header.Peek("Content-Encoding")

	var err error
	var body []byte
	switch string(encoding) {
	case "br":
		body, err = res.BodyUnbrotli()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"raw": string(res.Body()),
				"err": err,
			}).Error("failed to un-brotli body")
		}
	case "gzip":
		body, err = res.BodyGunzip()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"raw": string(res.Body()),
				"err": err,
			}).Error("failed to un-gzip body")
		}
	case "deflate":
		body, err = res.BodyInflate()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"raw": string(res.Body()),
				"err": err,
			}).Error("failed to inflate body")
		}
	default:
		body = res.Body()
	}

	if err != nil {
		return nil
	}

	return body
}
