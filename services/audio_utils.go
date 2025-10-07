package services

import (
	"io"
	"net/http"

	tcmp3 "github.com/tcolgate/mp3"
)

// Tính thời lượng file MP3 bằng URL, trả về số giây
func GetMP3DurationFromURL(url string) (float64, error) {

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var (
		dur     float64
		dec     = tcmp3.NewDecoder(resp.Body)
		frame   tcmp3.Frame
		skipped int
	)

	for {
		if err := dec.Decode(&frame, &skipped); err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}
		dur += frame.Duration().Seconds()
	}

	return dur, nil
}
