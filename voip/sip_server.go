package voip

import (
	"context"
	"fmt"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/pion/sdp/v3"
	"log"
	"strconv"
	"strings"
)

type SipServer struct {
	ua             *sipgo.UserAgent
	uas            *sipgo.Server
	regMap         map[string]sip.Uri
	dialogMap      map[string]*SipCall
	unicastAddress string
}

type SipCall struct {
	RemoteUser string
	Audio      *RtpServer
	Video      *RtpServer
	Client     *sipgo.Client
	Dialog     *sipgo.DialogClientSession
}

func (sc *SipCall) Close() error {
	sc.Audio.Close()
	sc.Video.Close()
	err := sc.Dialog.Bye(context.Background())
	if err != nil {
		return fmt.Errorf("Error sending BYE: %v", err)
	}
	err = sc.Client.Close()
	if err != nil {
		return fmt.Errorf("Error closing client: %v", err)
	}
	return nil
}

func NewSipServer(ua *sipgo.UserAgent, unicastAddress string) *SipServer {
	ss := &SipServer{
		ua:             ua,
		unicastAddress: unicastAddress,
	}
	var err error
	ss.uas, err = sipgo.NewServer(ua)
	if err != nil {
		panic(err)
	}

	ss.regMap = make(map[string]sip.Uri)
	ss.dialogMap = make(map[string]*SipCall)
	ss.uas.OnRegister(ss.HandleRegister)

	go ss.uas.ListenAndServe(context.Background(), "udp", ":5070")
	return ss
}

func (ss *SipServer) HandleRegister(req *sip.Request, tx sip.ServerTransaction) {
	contact := req.Contact()
	log.Printf("Source %s", req.Source())

	newAddress := contact.Address.Clone()
	fields := strings.Split(req.Source(), ":")
	newAddress.Host = fields[0]
	port, err := strconv.Atoi(fields[1])
	if err != nil {
		log.Printf("Error converting port: %v", err)
		return
	}
	newAddress.Port = port
	log.Printf("Registered %s to %s", req.From().Address.User, contact.Address)

	ss.regMap[req.From().Address.User] = *newAddress

	// Create a 200 OK response
	response := sip.NewResponseFromRequest(req, 200, "OK", nil)

	// Set the Expires header on the response
	response.AppendHeader(sip.NewHeader("Expires", "60"))

	_ = tx.Respond(response)
}

func (ss *SipServer) SendInvite(toUser string, correlationToken string, origSDP sdp.SessionDescription) (*SipCall, error) {
	uac, err := sipgo.NewClient(ss.ua, sipgo.WithClientHostname(ss.unicastAddress))
	if err != nil {
		return nil, fmt.Errorf("SendInvite: Error making new Client: %v", err)
	}

	audio, err := NewRtpServer("audio")
	if err != nil {
		return nil, fmt.Errorf("SendInvite: Error making new RTP server: %v", err)
	}

	video, err := NewRtpServer("video")
	if err != nil {
		audio.Close()
		return nil, fmt.Errorf("SendInvite: Error making new RTP server: %v", err)
	}

	sipCall := &SipCall{
		RemoteUser: toUser,
		Audio:      audio,
		Video:      video,
		Client:     uac,
	}

	ctx := context.Background()
	//req := sip.NewRequest("INVITE", ss.regMap[toUser])

	sdp := sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      origSDP.Origin.SessionID,
			SessionVersion: origSDP.Origin.SessionVersion,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: ss.unicastAddress,
		},
		SessionName: "Pion",
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: ss.unicastAddress},
		},
		TimeDescriptions: []sdp.TimeDescription{
			{
				Timing: sdp.Timing{
					StartTime: 0,
					StopTime:  0,
				},
			},
		},
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: audio.GetPort()},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				Attributes: []sdp.Attribute{
					{Key: "rtpmap", Value: "0 PCMU/8000"},
					{Key: "ptime", Value: "20"},
					{Key: "maxptime", Value: "150"},
					{Key: "sendrecv"},
				},
			},
			{
				MediaName: sdp.MediaName{
					Media:   "video",
					Port:    sdp.RangedPort{Value: video.GetPort()},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"99"},
				},
				Attributes: []sdp.Attribute{
					{Key: "rtpmap", Value: "99 H264/90000"},
					{Key: "fmtp", Value: "99 profile-level-id=42000a;packetization-mode=0"},
					{Key: "sendrecv"},
				},
			},
		},
	}

	sdpByte, err := sdp.Marshal()
	if err != nil {
		return nil, fmt.Errorf("SendInvite: Error marshalling SDP: %v", err)
	}

	contactHDR := sip.ContactHeader{
		Address: sip.Uri{User: "doorbell", Host: ss.unicastAddress, Port: 5088},
	}

	dialogCli := sipgo.NewDialogClientCache(uac, contactHDR)

	dialog, err := dialogCli.Invite(ctx, ss.regMap[toUser], sdpByte, sip.NewHeader("Content-Type", "application/sdp"))
	if err != nil {
		return nil, fmt.Errorf("SendInvite: Error sending INVITE: %v", err)
	}

	err = dialog.WaitAnswer(ctx, sipgo.AnswerOptions{})

	if err != nil {
		return nil, fmt.Errorf("SendInvite: Error waiting for answer: %v", err)
	}

	err = dialog.Ack(ctx)
	if err != nil {
		return nil, fmt.Errorf("SendInvite: Error sending ACK: %v", err)
	}

	sipCall.Dialog = dialog

	dialog.OnState(func(s sip.DialogState) {
		fmt.Errorf("Dialog state: %v", s)
	})

	ss.dialogMap[correlationToken] = sipCall
	defer func() {
		if err != nil {
			if uac != nil {
				_ = uac.Close()
			}
			if audio != nil {
				audio.Close()
			}
			if video != nil {
				video.Close()
			}
		}
	}()

	return sipCall, nil
}

func (ss *SipServer) SendBye(correlationToken string) error {
	sipCall, ok := ss.dialogMap[correlationToken]
	if !ok {
		return fmt.Errorf("SendBye: Call not found")
	}

	err := sipCall.Close()
	if err != nil {
		return fmt.Errorf("SendBye: Error clearing call: %v", err)
	}
	fmt.Printf("Sent BYE to %s\n", sipCall.RemoteUser)

	delete(ss.dialogMap, correlationToken)
	return nil
}
