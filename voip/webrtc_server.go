package voip

import (
	"fmt"
	"github.com/pion/webrtc/v4"
	"net"
	"time"
)

type RTCServer struct {
	dialogMap map[string]*RTCCall
}

type RTCCall struct {
	localAudioTrack   *webrtc.TrackLocalStaticRTP
	localVideoTrack   *webrtc.TrackLocalStaticRTP
	remoteAudioTrack  *webrtc.TrackRemote
	remoteVideoTrack  *webrtc.TrackRemote
	audioWriteHandler func(payload []byte)
	videoWriteHandler func(payload []byte)
	peerConnection    *webrtc.PeerConnection
	shutdown          bool
}

func (r *RTCCall) ReadRTCP(track *webrtc.RTPSender) {
	for !r.shutdown {
		track.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		select {
		default:
			rtpBuf := make([]byte, 1500)
			_, _, err := track.Read(rtpBuf)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if err != nil {
				fmt.Printf("Error reading from track: %v\n", err)
			}
		}
	}
	fmt.Printf("Shutting down RTCRead\n")
}

func (r *RTCCall) Bridge(track webrtc.TrackRemote, writeHandler func(payload []byte)) {
	for !r.shutdown {
		select {
		default:
			track.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			rtpBuf := make([]byte, 1500)
			_, _, err := track.Read(rtpBuf)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if err != nil {
				fmt.Printf("Error reading from track: %v\n", err)
			}

			if writeHandler != nil {
				writeHandler(rtpBuf)
			}
		}
	}
	fmt.Printf("Shutting down Bridge\n")
}

func (r *RTCCall) Close() {
	r.shutdown = true
	_ = r.peerConnection.Close()
}

func (r *RTCCall) SetAudioWriteHandler(handler func(payload []byte)) {
	r.audioWriteHandler = handler
	//go r.Bridge(r.remoteAudioTrack, handler)
}

func (r *RTCCall) SetVideoWriteHandler(handler func(payload []byte)) {
	r.videoWriteHandler = handler
	//go r.Bridge(r.remoteVideoTrack, handler)
}

func (r *RTCCall) WriteAudio(payload []byte) (int, error) {
	return r.localAudioTrack.Write(payload)
}

func (r *RTCCall) WriteVideo(payload []byte) (int, error) {
	return r.localVideoTrack.Write(payload)
}

func NewRTCServer() *RTCServer {
	r := &RTCServer{
		dialogMap: make(map[string]*RTCCall),
	}

	return r
}

func (r *RTCServer) NewCall(correlationToken string, sdpStr string) (*RTCCall, string, error) {
	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, "", fmt.Errorf("NewCall: Error creating new PeerConnection: %v", err)
	}

	call := &RTCCall{
		peerConnection: peerConnection,
		shutdown:       false,
	}

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			fmt.Printf("Got audio track: %s\n", track.Codec().MimeType)
			call.remoteAudioTrack = track
		} else if track.Kind() == webrtc.RTPCodecTypeVideo {
			fmt.Printf("Got video track: %s\n", track.Codec().MimeType)
			call.remoteVideoTrack = track
		}
	})

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	})

	// Create a audio track
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU}, "audio", "pion-audio")
	if err != nil {
		panic(err)
	}
	rtpAudioSender, err := peerConnection.AddTrack(audioTrack)
	if err != nil {
		panic(err)
	}
	call.localAudioTrack = audioTrack

	// Create a audio track
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion-video")
	if err != nil {
		panic(err)
	}
	rtpVideoSender, err := peerConnection.AddTrack(videoTrack)
	if err != nil {
		panic(err)
	}
	call.localVideoTrack = videoTrack

	sdpOffer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdpStr,
	}

	if err = peerConnection.SetRemoteDescription(sdpOffer); err != nil {
		return nil, "", fmt.Errorf("NewCall: Error setting remote description: %v", err)
	}

	// Create an answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	if err = peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}

	<-gatherComplete

	go call.ReadRTCP(rtpAudioSender)
	go call.ReadRTCP(rtpVideoSender)

	r.dialogMap[correlationToken] = call
	return call, peerConnection.LocalDescription().SDP, nil
}

func (r *RTCServer) HangupCall(correlationToken string) error {
	call, ok := r.dialogMap[correlationToken]
	if !ok {
		return fmt.Errorf("HangupCall: Call not found")
	}

	call.Close()
	delete(r.dialogMap, correlationToken)
	return nil
}
