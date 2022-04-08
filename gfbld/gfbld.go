package gfbld

import (
	"GFBLD/gfbld/database"
	"GFBLD/gfbld/fetcher"
	"context"
	"fmt"
	"github.com/cavaliergopher/grab/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gorm.io/gorm"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Downloader struct {
	DB         *gorm.DB
	Fetcher    *fetcher.Fetcher
	stopSignal chan int
}

func NewDownloader() *Downloader {
	logrus.Info("creating downloader")
	path := viper.GetString("sqlite_path")
	if len(path) == 0 {
		logrus.Panic("empty sqlite_path")
	}
	useProxy := viper.GetBool("use_proxy")
	var proxy string
	if useProxy {
		proxy = viper.GetString("proxy")
		if len(proxy) == 0 {
			logrus.Panic("empty proxy url")
		}
	}
	id := viper.GetInt("id")
	if id == 0 {
		logrus.Panic("empty id")
	}
	vlDocId := viper.GetInt("video_list_doc_id")
	if vlDocId == 0 {
		logrus.Panic("empty video_list_doc_id")
	}
	viDocId := viper.GetInt("video_info_doc_id")
	if viDocId == 0 {
		logrus.Panic("empty video_info_doc_id")
	}
	return &Downloader{
		DB:         database.NewDB(path),
		Fetcher:    fetcher.NewFetcher(proxy, id, vlDocId, viDocId),
		stopSignal: make(chan int, 1),
	}
}

func (d *Downloader) Start() {
	logrus.Info("start the downloader loop")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			logrus.Info("downloader tick")
			ll := d.Fetcher.FetchLiveList()
			for _, l := range ll {
				if err := d.DB.Where("live_id = ?", l.LiveId).First(&database.LiveRecord{}).Error; err == nil {
					continue
				}
				d.DB.Create(&l)
			}
			ll = d.getFilteredLiveList()

			d.Fetcher.FetchLiveInfo(ll, d.DB)
			d.download(ll)
			d.combine()
		case <-d.stopSignal:
			logrus.Info("stop the downloader loop")
			return
		}
	}
}

func (d *Downloader) Stop() {
	logrus.Info("stop the downloader")
	d.stopSignal <- 1
}

func (d *Downloader) getFilteredLiveList() []*database.LiveRecord {
	logrus.Info("get filtered live list from database")
	ll := make([]*database.LiveRecord, 0)
	if err := d.DB.Find(&ll, "combined = ?", false).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			logrus.Warn("no available live record yet")
		} else {
			logrus.WithError(err).Error("failed to retrieve live record")
		}
	}
	return ll
}

func (d *Downloader) download(ll []*database.LiveRecord) {
	logrus.Info("start download")
	_, err := os.Stat("downloaded")
	if err != nil {
		if err := os.MkdirAll(filepath.Join(".", "downloaded"), os.ModePerm); err != nil {
			logrus.WithError(err).Panic("failed to create downloaded folder")
		}
	}
	proxy, err := url.Parse(fmt.Sprintf("http://%s", viper.GetString("proxy")))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"proxy": viper.GetString("proxy"),
			"err":   err,
		}).Panic("failed to parse proxy url")
	}

	for _, l := range ll {
		v := &database.Video{}
		a := &database.Audio{}
		if err := d.DB.Where("belongs_to = ?", l.LiveId).First(&v).Error; err != nil {
			logrus.WithError(err).Error("failed to get video url")
			continue
		}
		if err := d.DB.Where("belongs_to = ?", l.LiveId).First(&a).Error; err != nil {
			logrus.WithError(err).Error("failed to get audio url")
			continue
		}

		// Download video
		gClient, gReq, err := d.createGrab(proxy, v.Url)
		if err != nil {
			logrus.WithError(err).Error("failed to create grab")
		}
		res := gClient.Do(gReq)
		ticker := time.NewTicker(250 * time.Millisecond)

		for {
			select {
			case <-ticker.C:
				fmt.Printf("downloading: %s, progress: %.2f%%, eta: %s\r", v.Name, 100*res.Progress(), res.ETA().Format("15:04:05"))
			case <-res.Done:
				if err := res.Err(); err != nil {
					if strings.Contains(err.Error(), "403 Forbidden") {
						logrus.WithField("name", v.Name).Warn("video url expired, clear")
						d.DB.Model(&database.Video{}).Where("belongs_to = ?", v.BelongsTo).Update("url", "")
					}
					logrus.WithError(err).Error("download failed")
				} else {
					logrus.WithField("name", v.Name).Info("video download complete")
					v.FileName = res.Filename
					v.Downloaded = true
					d.DB.Save(&v)
				}
				goto BreakVideoLoop
			}
		}
	BreakVideoLoop:

		// Download audio
		gClient, gReq, err = d.createGrab(proxy, a.Url)
		if err != nil {
			logrus.WithError(err).Error("failed to create grab")
		}
		res = gClient.Do(gReq)
		ticker = time.NewTicker(250 * time.Millisecond)

		for {
			select {
			case <-ticker.C:
				fmt.Printf("downloading: %s, progress: %.2f%%, eta: %s\r", a.Name, 100*res.Progress(), res.ETA().Format("15:04:05"))
			case <-res.Done:
				if err := res.Err(); err != nil {
					if strings.Contains(err.Error(), "403 Forbidden") {
						logrus.WithField("name", a.Name).Warn("audio url expired, clear")
						d.DB.Model(&database.Audio{}).Where("belongs_to = ?", a.BelongsTo).Update("url", "")
					}
					logrus.WithError(err).Error("download failed")
				} else {
					logrus.WithField("name", a.Name).Info("audio download complete")
					a.FileName = res.Filename
					a.Downloaded = true
					d.DB.Save(&a)
				}
				goto BreakAudioLoop
			}
		}
	BreakAudioLoop:
	}
}

func (d *Downloader) combine() {
	logrus.Info("start combine")
	vl := make([]*database.Video, 0)
	if err := d.DB.Where("downloaded = ?", true).Find(&vl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			logrus.Warn("no downloaded video yet")
		} else {
			logrus.WithError(err).Error("failed to retrieve downloaded video")
		}
		return
	}
	al := make([]*database.Audio, 0)
	if err := d.DB.Where("downloaded = ?", true, false).Find(&al).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			logrus.Warn("no downloaded audio yet")
		} else {
			logrus.WithError(err).Error("failed to retrieve downloaded video")
		}
		return
	}
	ll := make([]*database.LiveRecord, 0)
	if err := d.DB.Where("combined = ?", false).Find(&ll).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			logrus.Warn("no live record yet")
		} else {
			logrus.WithError(err).Error("failed to retrieve live record")
		}
		return
	}

	for _, l := range ll {
		logrus.Infof("combining %s", l.Name)
		var videoFilename, audioFilename, finalName string
		finalName = fmt.Sprintf("%s.mp4", l.Name)
		for _, v := range vl {
			if v.BelongsTo == l.LiveId {
				videoFilename = v.FileName
				break
			}
		}
		for _, a := range al {
			if a.BelongsTo == l.LiveId {
				audioFilename = a.FileName
				break
			}
		}

		if len(audioFilename) == 0 || len(videoFilename) == 0 {
			logrus.WithField("name", finalName).Warn("audio or video haven't downloaded yet, skip")
			continue
		}
		cmd := exec.Command("ffmpeg", "-loglevel", "quiet", "-i", videoFilename, "-i", audioFilename, "-vcodec", "copy", "-acodec", "copy", "-y", fmt.Sprintf("downloaded/%s", finalName))
		out, err := cmd.Output()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"err": err,
				"out": string(out),
			}).Error("failed to combine video and audio")
			continue
		}
		logrus.WithField("output", string(out)).Info("combined, remove temp file")
		_ = os.Remove(videoFilename)
		_ = os.Remove(audioFilename)

		l.Combined = true
		d.DB.Save(&l)
	}
}

func (d *Downloader) createGrab(proxy *url.URL, url string) (*grab.Client, *grab.Request, error) {
	logrus.Info("create grab client")
	gClient := &grab.Client{
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.55 Safari/537.36",
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxy),
			},
		},
	}
	gReq, err := grab.NewRequest("./downloaded", url)
	if err != nil {
		return nil, nil, err
	}
	if viper.GetInt("speed_limit") == 0 {
		logrus.Warn("speed_limit is 0, will download at full-speed!")
	} else {
		gReq.RateLimiter = newRateLimiter(viper.GetInt("speed_limit"))
	}
	return gClient, gReq, nil
}

type rateLimiter struct {
	r, n int
}

func newRateLimiter(rate int) rateLimiter {
	return rateLimiter{r: rate}
}

func (r rateLimiter) WaitN(ctx context.Context, n int) error {
	r.n += n
	time.Sleep(
		time.Duration(1.00 / float64(r.r) * float64(n) * float64(time.Second)))
	return nil
}
