import { Facebook, Instagram, Mail, MessageSquare, MessagesSquare, Phone, Send } from 'lucide-vue-next'

// Icon per inbox channel, so a conversation is identifiable at a glance in the
// list and the sidebar. Falls back to Mail for the email channel.
const CHANNEL_ICONS = {
  livechat: MessageSquare,
  whatsapp: MessagesSquare,
  messenger: Facebook,
  instagram: Instagram,
  telegram: Send,
  voice: Phone
}

export const channelIcon = (channel) => CHANNEL_ICONS[channel] || Mail
