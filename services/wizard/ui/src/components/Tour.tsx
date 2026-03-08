import { useState } from 'react'

interface TourProps {
  onClose: () => void
}

const STEPS = [
  {
    title: 'Welcome to SafePaw',
    body: `SafePaw gives your team a **private AI assistant** — like ChatGPT, but running on YOUR servers. No one else sees your conversations.

This control panel lets you manage everything without touching any code.`,
    emoji: '🐾',
  },
  {
    title: 'The Home Page',
    body: `**Home** shows you everything at a glance:

• **Green dots** = things are running fine
• **Numbers at the top** = how many people are using the AI and whether anything suspicious happened
• **"Chat with AI"** button = opens the AI assistant in a new tab

Each card is a part of the system: the AI itself, the security layer that protects it, and the databases that remember things.`,
    emoji: '🏠',
  },
  {
    title: 'Security Monitor',
    body: `The **Security** tab is your security camera.

• **Conversations** = how many messages have been sent
• **Attacks Stopped** = someone tried to trick or hack the AI and we caught it
• **Spam Blocked** = someone was sending too many messages too fast
• **Failed Logins** = someone tried to get in without the right credentials

Yellow numbers = something you should look at. All zeros = everything's fine.`,
    emoji: '🛡️',
  },
  {
    title: 'Settings',
    body: `**Settings** is where you control everything:

• **AI Provider** — paste your API key from Anthropic or OpenAI
• **Security** — turn login on/off, change passwords
• **Spam Protection** — limit how fast people can send messages
• **Encryption** — turn on HTTPS for secure connections
• **Chat Channels** — connect to Slack, Discord, or Telegram

Toggle switches for on/off options. Hit "Save" when you're done. Some changes need a service restart (you can do that from Home).`,
    emoji: '⚙️',
  },
  {
    title: '"Chat with AI" Button',
    body: `The green **"Chat with AI"** button on the Home page opens your private AI assistant in a new browser tab.

Behind the scenes, it creates a temporary access pass that expires in 1 hour. Your team gets a secure chat interface — no API keys, no setup, just a conversation.

This is what you'd show your team: "Click this button, start chatting."`,
    emoji: '💬',
  },
  {
    title: 'What Makes This Different?',
    body: `**vs. ChatGPT / Claude directly:**
• Your data stays on your servers — not on OpenAI's or Anthropic's
• Every message is scanned for attacks before reaching the AI
• You see exactly who's using it and how much

**vs. doing nothing:**
• Right now, your team is probably copy-pasting sensitive data into public AI tools
• You have zero visibility into what they're asking
• SafePaw gives you control without slowing anyone down`,
    emoji: '✨',
  },
  {
    title: 'You\'re Ready!',
    body: `That's everything. Three pages:

1. **Home** — is everything running?
2. **Security** — is anything suspicious happening?
3. **Settings** — change how it works

Click the **?** button in the top right any time to see this tour again.`,
    emoji: '🎉',
  },
]

export function Tour({ onClose }: TourProps) {
  const [step, setStep] = useState(0)
  const current = STEPS[step]!
  const isLast = step === STEPS.length - 1
  const isFirst = step === 0

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center p-4">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={onClose} />

      {/* Card */}
      <div className="relative bg-gray-900 border border-gray-700 rounded-2xl shadow-2xl max-w-lg w-full p-8 page-enter">
        {/* Close */}
        <button
          onClick={onClose}
          className="absolute top-4 right-4 w-8 h-8 rounded-full text-gray-500 hover:text-white hover:bg-gray-800 transition-colors flex items-center justify-center"
        >
          ×
        </button>

        {/* Progress dots */}
        <div className="flex justify-center gap-1.5 mb-6">
          {STEPS.map((_, i) => (
            <div
              key={i}
              className={`h-1.5 rounded-full transition-all duration-300 ${
                i === step ? 'w-6 bg-paw-500' : i < step ? 'w-1.5 bg-paw-600/50' : 'w-1.5 bg-gray-700'
              }`}
            />
          ))}
        </div>

        {/* Emoji */}
        <div className="text-center mb-4">
          <span className="text-5xl">{current.emoji}</span>
        </div>

        {/* Title */}
        <h3 className="text-xl font-bold text-center mb-4">{current.title}</h3>

        {/* Body — render markdown-like bold */}
        <div className="text-sm text-gray-300 leading-relaxed space-y-2">
          {current.body.split('\n\n').map((paragraph, pi) => (
            <p key={pi}>
              {paragraph.split('\n').map((line, li) => (
                <span key={li}>
                  {li > 0 && <br />}
                  {renderBold(line)}
                </span>
              ))}
            </p>
          ))}
        </div>

        {/* Navigation */}
        <div className="flex items-center justify-between mt-8">
          <button
            onClick={() => setStep(s => s - 1)}
            disabled={isFirst}
            className="btn-secondary text-sm py-2 px-4 disabled:opacity-0"
          >
            ← Back
          </button>
          <span className="text-xs text-gray-500">
            {step + 1} of {STEPS.length}
          </span>
          {isLast ? (
            <button onClick={onClose} className="btn-primary text-sm py-2 px-4">
              Done ✓
            </button>
          ) : (
            <button onClick={() => setStep(s => s + 1)} className="btn-primary text-sm py-2 px-4">
              Next →
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

/** Simple bold renderer: **text** → <strong>text</strong> */
function renderBold(text: string) {
  const parts = text.split(/(\*\*[^*]+\*\*)/)
  return parts.map((part, i) => {
    if (part.startsWith('**') && part.endsWith('**')) {
      return <strong key={i} className="text-white font-semibold">{part.slice(2, -2)}</strong>
    }
    return <span key={i}>{part}</span>
  })
}
