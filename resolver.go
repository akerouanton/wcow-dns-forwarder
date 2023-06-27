package main

import (
	"context"
	"fmt"
	"github.com/containerd/containerd/log"
	"net"
	"time"

	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

const (
	maxConcurrent = 1024
	logInterval   = 2 * time.Second
)

// Resolver is the embedded DNS server in Docker. It operates by listening on
// the container's loopback interface for DNS queries.
type Resolver struct {
	udpServer     *dns.Server
	udpConn       *net.UDPConn
	tcpServer     *dns.Server
	tcpConn       *net.TCPListener
	listenAddress string
	logger        *logrus.Logger

	fwdSem      *semaphore.Weighted // Limit the number of concurrent external DNS requests in-flight
	logInverval rate.Sometimes      // Rate-limit logging about hitting the fwdSem limit
}

// NewResolver creates a new instance of the Resolver
func NewResolver(address string) *Resolver {
	return &Resolver{
		listenAddress: address,
		logger:        log.G(context.TODO()).Logger,
		logInverval:   rate.Sometimes{Interval: logInterval},
	}
}

// Start starts the name server for the container.
func (r *Resolver) Start() error {
	var err error

	// DNS operates primarily on UDP
	if r.udpConn, err = net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP(r.listenAddress),
		Port: 53,
	}); err != nil {
		return fmt.Errorf("error in opening name server socket %v", err)
	}

	r.udpServer = &dns.Server{Handler: dns.HandlerFunc(r.serveDNS), PacketConn: r.udpConn}
	go func() {
		if err := r.udpServer.ActivateAndServe(); err != nil {
			r.logger.WithError(err).Error("[resolver] failed to start PacketConn DNS server")
		}
	}()

	// Listen on a TCP as well
	if r.tcpConn, err = net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.ParseIP(r.listenAddress),
		Port: 53,
	}); err != nil {
		return fmt.Errorf("error in opening name TCP server socket %v", err)
	}

	r.tcpServer = &dns.Server{Handler: dns.HandlerFunc(r.serveDNS), Listener: r.tcpConn}
	go func() {
		if err := r.tcpServer.ActivateAndServe(); err != nil {
			r.logger.WithError(err).Error("[resolver] failed to start TCP DNS server")
		}
	}()

	return nil
}

// Stop stops the name server for the container. A stopped resolver can be
// reused after running the SetupFunc again.
func (r *Resolver) Stop() {
	if r.udpServer != nil {
		r.udpServer.Shutdown()
		r.udpServer = nil
	}
	if r.tcpServer != nil {
		r.tcpServer.Shutdown() //nolint:errcheck
		r.tcpServer = nil
	}

	r.udpConn = nil
	r.tcpConn = nil
}

func (r *Resolver) serveDNS(w dns.ResponseWriter, query *dns.Msg) {
	if query == nil || len(query.Question) == 0 {
		return
	}

	resp, err := r.forward(query)
	if err != nil {
		r.logger.WithFields(map[string]interface{}{
			logrus.ErrorKey: err,
			"query":         query,
		}).Error("failed to forward request")
	}
	if resp == nil {
		// We were unable to get an answer from any of the upstream DNS servers.
		resp = new(dns.Msg).SetRcode(query, dns.RcodeServerFailure)
	}
	if err = w.WriteMsg(resp); err != nil {
		r.logger.WithError(err).Errorf("[resolver] failed to write response")
	}
}
