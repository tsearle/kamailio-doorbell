package voip

import (
	"fmt"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"net"
	"strings"
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
				fmt.Printf("Error reading from RTCP track: %v\n", err)
			}
		}
	}
	fmt.Printf("Shutting down RTCRead\n")
}

func (r *RTCCall) BridgeAudio() {
	for !r.shutdown {
		select {
		default:
			if r.remoteAudioTrack != nil {
				r.remoteAudioTrack.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				rtpBuf := make([]byte, 1500)
				n, _, err := r.remoteAudioTrack.Read(rtpBuf)
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if err != nil {
					fmt.Printf("Error reading from AUDIO track: %v\n", err)
				}

				if r.audioWriteHandler != nil {
					r.audioWriteHandler(rtpBuf[:n])
				}
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	fmt.Printf("Shutting down Bridge\n")
}

func (r *RTCCall) BridgeVideo() {
	for !r.shutdown {
		select {
		default:
			if r.remoteVideoTrack != nil {
				r.remoteVideoTrack.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				rtpBuf := make([]byte, 1500)
				n, _, err := r.remoteVideoTrack.Read(rtpBuf)
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if err != nil {
					fmt.Printf("Error reading from VIDEO track: %v\n", err)
				}

				if r.videoWriteHandler != nil {
					r.videoWriteHandler(rtpBuf[:n])
				}
			} else {
				time.Sleep(100 * time.Millisecond)
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
}

func (r *RTCCall) SetVideoWriteHandler(handler func(payload []byte)) {
	r.videoWriteHandler = handler
}

func (r *RTCCall) WriteAudio(payload []byte) (int, error) {
	return r.localAudioTrack.Write(payload)
}

func (r *RTCCall) WriteVideo(payload []byte) (int, error) {
	return r.localVideoTrack.Write(payload)
}
func filterAudio(media *sdp.MediaDescription) {
	media.MediaName.Formats = []string{"0"}
	filtered := []sdp.Attribute{}
	for _, attribute := range media.Attributes {
		if strings.HasPrefix(attribute.Key, "rtpmap") {
			if strings.Contains(attribute.Value, "PCMU") {
				filtered = append(filtered, attribute)
				break
			}
		} else {
			filtered = append(filtered, attribute)
		}
	}
	media.Attributes = filtered
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

	var parsedSDP sdp.SessionDescription
	if err := parsedSDP.Unmarshal([]byte(sdpStr)); err != nil {
		return nil, "", fmt.Errorf("NewCall: Error parsing SDP: %v", err)
	}

	// Filter codecs we only want pcmu
	//filteredMediaDescriptions := []*sdp.MediaDescription{}
	for _, media := range parsedSDP.MediaDescriptions {
		// Only keep codecs you want (example: keep only Opus and VP8)
		if media.MediaName.Media == "audio" {
			filterAudio(media)
		}
	}
	//parsedSDP.MediaDescriptions = filteredMediaDescriptions

	// Marshal the modified SDP back to a string
	modifiedSDP, err := parsedSDP.Marshal()
	if err != nil {
		return nil, "", fmt.Errorf("NewCall: Error marshalling modified SDP: %v", err)
	}

	sdpOffer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  string(modifiedSDP),
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
	go call.BridgeAudio()
	go call.BridgeVideo()

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
