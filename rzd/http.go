package rzd

import (
	"net/http"
	"net/url"
)

type AddHeaderTransport struct {
	T http.RoundTripper
}

func (adt *AddHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	params := url.Values{}
	params.Encode()
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Pragma", "no-cache")
	req.Header.Add("Accept", "application/json, text/plain, */*")
	req.Header.Add("Accept-Language", "en-GB,en;q=0.9")
	req.Header.Add("Accept-Encoding", "deflate")
	req.Header.Add("Cache-Control", "no-cache")
	req.Header.Add("Origin", "https://ticket.rzd.ru")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15")
	req.Header.Add("Referer", "https://ticket.rzd.ru/searchresults/v/1/5a13ba4b340c745ca1e7dda0/5a3244bc340c7441a0a556ca/2023-04-02")
	req.Header.Add("Cookie", "_ga=GA1.1.118455456.1678553833; _ga_J932X5NKG9=GS1.1.1678563767.3.0.1678563767.0.0.0; _ym_visorc=b; LANG_SITE=ru; _gid=GA1.2.409377037.1678553833; accessible=false; _ym_isad=2; session-cookie=174b6bd11685cf21046c7a5be59e4e026eb27a5669e9bdf8cd4d5e1aca896abf44a23f51c2dee2da12b5a6e73db142e4; lang=ru; acceptCookies=1; jsonFormat=true; _ym_d=1678212254; _ym_uid=1678212254580970909")
	req.Header.Add("sentry-trace", "5ba9af2e58b042a19c40ea02a4ca554f-abd925811d5d2cb0-1")
	return adt.T.RoundTrip(req)
}
