package models

// Action constants for WebSocket messages.
const (
	MessageTypeMessageUpdate         = "message_update"
	MessageTypeConversationUpdate    = "conversation_update"
	MessageTypeNewMessage            = "new_message"
	MessageTypeNewConversation       = "new_conversation"
	MessageTypeNewNotification       = "new_notification"
	MessageTypeError                 = "error"
	MessageTypeConversationSubscribe = "conversation_subscribe"
	MessageTypeTyping                = "typing"
	MessageTypeListSubscribeReplace  = "list_subscribe_replace"
	MessageTypeAgentAvailability     = "agent_availability_update"
	// An incoming phone call is ringing agents, and has stopped.
	MessageTypeIncomingCall      = "incoming_call"
	MessageTypeIncomingCallEnded = "incoming_call_ended"
	// The call-backs changed: one opened, was added to, or closed.
	MessageTypeCallBacksChanged = "call_backs_changed"
	// A contact moved to another lead stage.
	MessageTypeLeadStageChanged = "lead_stage_changed"
)

// WSMessage represents a WS message.
type WSMessage struct {
	MessageType int
	Data        []byte
}

// Message represents a WebSocket message to be sent.
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// BroadcastMessage represents a message to be pushed to users.
type BroadcastMessage struct {
	Data  []byte `json:"data"`
	Users []int  `json:"users"`
	// TenantID scopes a broadcast without Users to the tenant's agents.
	TenantID int `json:"tenant_id"`
}

// ConversationSubscribe represents a conversation subscription message.
type ConversationSubscribe struct {
	ConversationUUID string `json:"conversation_uuid"`
}

// TypingMessage represents a typing indicator message.
type TypingMessage struct {
	ConversationUUID string `json:"conversation_uuid"`
	IsTyping         bool   `json:"is_typing"`
	IsPrivateMessage bool   `json:"is_private_message"`
}
