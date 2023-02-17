package udxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/pion/rtp"
)

const (
	RTP_Payload_MP2T = 33
)

const (
	// https://www.w3.org/2013/12/byte-stream-format-registry/mp2t-byte-stream-format.html
	ContentType_MP2T    = "video/MP2T"
	ContentType_DEFAULT = "application/octet-stream"
)

func init() {
	caddy.RegisterModule(Udpxy{})
	httpcaddyfile.RegisterHandlerDirective("udpxy", parseCaddyfile)
}

type Udpxy struct {
	InterfaceName string `json:"interface"`
	Timeout       string `json:"timeout"`
	inteface      *net.Interface
	timeout       time.Duration
}

func (Udpxy) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.udpxy",
		New: func() caddy.Module { return new(Udpxy) },
	}

}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	u := new(Udpxy)
	err := u.UnmarshalCaddyfile(h.Dispenser)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (u *Udpxy) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if d.NextArg() {
			return d.ArgErr()
		}
		for nesting := d.Nesting(); d.NextBlock(nesting); {
			switch d.Val() {
			case "interface":
				if d.NextArg() {
					u.InterfaceName = d.Val()
				}
			case "timeout":
				if d.NextArg() {
					u.Timeout = d.Val()
				}
			}

		}
	}
	fmt.Println("dd")
	fmt.Println(u.InterfaceName)
	return nil
}

func (u *Udpxy) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	var wc int64
	parts := strings.FieldsFunc(r.URL.Path, func(r rune) bool { return r == '/' })
	if len(parts) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "No address specified")
		return next.ServeHTTP(w, r)
	}
	raddr := parts[1]
	addr, err := net.ResolveUDPAddr("udp4", raddr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, err.Error())
		return next.ServeHTTP(w, r)
	}

	conn, err := net.ListenMulticastUDP("udp4", u.inteface, addr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		return next.ServeHTTP(w, r)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add((u.timeout)))
	var buf = make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		return next.ServeHTTP(w, r)
	}
	conn.SetReadDeadline(time.Time{})
	p := &rtp.Packet{}
	headerSent := false
	for {
		if err = p.Unmarshal(buf[:n]); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, err.Error())
			return next.ServeHTTP(w, r)

		}

		if !headerSent {
			headerSent = true
			if p.PayloadType == RTP_Payload_MP2T {
				w.Header().Set("Content-Type", ContentType_MP2T)
			} else {
				w.Header().Set("Content-Type", ContentType_DEFAULT)
			}
			w.WriteHeader(http.StatusOK)
		}

		if _, werr := w.Write(p.Payload); werr != nil {
			break
		} else {
			wc += int64(n)
		}

		if n, err = conn.Read(buf); err != nil {
			break
		}
	}
	if err != nil && err != io.EOF {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		return next.ServeHTTP(w, r)
	}
	return next.ServeHTTP(w, r)
}

func (u *Udpxy) Validate() error {
	if u.inteface == nil {
		return fmt.Errorf("no interface")
	}
	return nil
}

func (u *Udpxy) Provision(ctx caddy.Context) error {
	inf, err := net.InterfaceByName(u.InterfaceName)
	if err != nil {
		return err
	}
	u.inteface = inf
	timeout, err := time.ParseDuration(u.Timeout)
	if err != nil {
		return err
	}
	u.timeout = timeout
	return nil
}

// Interface guards
var (
	_ caddy.Provisioner           = (*Udpxy)(nil)
	_ caddy.Validator             = (*Udpxy)(nil)
	_ caddyhttp.MiddlewareHandler = (*Udpxy)(nil)
	_ caddyfile.Unmarshaler       = (*Udpxy)(nil)
)
