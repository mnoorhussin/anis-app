import { PLANS, type PlanId } from '@anis/types';
import { Badge, Button, Card } from '@anis/ui';
import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react';

import type { StringKey } from '../i18n.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';

type ConversationStatus = 'active' | 'auto_resolved' | 'escalated' | 'human' | 'closed';

interface Conversation {
  id: string;
  status: ConversationStatus;
  language: 'ar' | 'en' | '';
  created: string;
  updated: string;
}

interface ChatMessage {
  id: string;
  conversation: string;
  role: 'user' | 'assistant' | 'human';
  text: string;
  outcome: string;
  created: string;
}

interface Lead {
  id: string;
  conversation: string;
  name: string;
  contact: string;
  contact_kind: string;
}

const STATUS_LABEL: Record<ConversationStatus, StringKey> = {
  active: 'statusActive',
  auto_resolved: 'statusAutoResolved',
  escalated: 'statusEscalated',
  human: 'statusHuman',
  closed: 'statusClosed',
};

const STATUS_TONE = {
  active: 'info',
  auto_resolved: 'success',
  escalated: 'warning',
  human: 'brand',
  closed: 'neutral',
} as const;

/** Conversations a person needs to look at, before anything else. */
const NEEDS_ATTENTION: ConversationStatus[] = ['escalated', 'human'];

export function InboxScreen({ workspaceId, plan }: { workspaceId: string; plan: PlanId }) {
  const { t, lang } = useLanguage();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [leads, setLeads] = useState<Lead[]>([]);
  const [error, setError] = useState<string | null>(null);

  const canTakeOver = PLANS[plan].features.includes('humanTakeover');

  const loadConversations = useCallback(() => {
    // No workspace filter: the collection rules already scope this. The sort
    // puts anything waiting for a person first — an inbox ordered purely by
    // time buries the one conversation that actually needs someone.
    pb.collection('conversations')
      .getFullList<Conversation>({ sort: '-updated' })
      .then((all) =>
        setConversations(
          [...all].sort((a, b) => {
            const aWaiting = NEEDS_ATTENTION.includes(a.status) ? 0 : 1;
            const bWaiting = NEEDS_ATTENTION.includes(b.status) ? 0 : 1;
            return aWaiting - bWaiting || b.updated.localeCompare(a.updated);
          }),
        ),
      )
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)));
  }, []);

  useEffect(loadConversations, [loadConversations]);

  // Realtime. An agent watching the inbox must see a visitor's message as it
  // arrives — polling would make "take over and reply" feel broken.
  useEffect(() => {
    let unsubMessages: (() => void) | undefined;
    let unsubConversations: (() => void) | undefined;

    void pb
      .collection('messages')
      .subscribe<ChatMessage>('*', (ev) => {
        setMessages((prev) =>
          ev.action === 'create' && prev.some((m) => m.conversation === ev.record.conversation)
            ? [...prev, ev.record]
            : prev,
        );
        loadConversations();
      })
      .then((fn) => {
        unsubMessages = fn;
      });

    void pb
      .collection('conversations')
      .subscribe('*', () => loadConversations())
      .then((fn) => {
        unsubConversations = fn;
      });

    return () => {
      unsubMessages?.();
      unsubConversations?.();
    };
  }, [loadConversations]);

  // Load the selected conversation's messages and any lead attached to it.
  useEffect(() => {
    if (!selected) {
      setMessages([]);
      setLeads([]);
      return;
    }
    void pb
      .collection('messages')
      .getFullList<ChatMessage>({ filter: `conversation="${selected}"`, sort: 'created' })
      .then(setMessages)
      .catch(() => setMessages([]));
    void pb
      .collection('leads')
      .getFullList<Lead>({ filter: `conversation="${selected}"` })
      .then(setLeads)
      .catch(() => setLeads([]));
  }, [selected]);

  async function setStatus(status: ConversationStatus) {
    if (!selected) return;
    try {
      await pb.collection('conversations').update(selected, { status });
      loadConversations();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  const current = conversations.find((c) => c.id === selected) ?? null;

  return (
    <div className="grid gap-4 md:grid-cols-[18rem_1fr]">
      <Card className="max-h-[32rem] overflow-y-auto">
        {error && (
          <p role="alert" className="mb-2 text-sm text-danger">
            {error}
          </p>
        )}
        {conversations.length === 0 ? (
          <p className="text-sm text-muted">{t('inboxEmpty')}</p>
        ) : (
          <ul className="flex flex-col gap-1">
            {conversations.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => setSelected(c.id)}
                  className={`flex w-full flex-col items-start gap-1 rounded-xl px-3 py-2 text-start transition-colors ${
                    c.id === selected ? 'bg-surface-3' : 'hover:bg-surface-2'
                  }`}
                >
                  <span className="flex w-full items-center gap-2">
                    <Badge tone={STATUS_TONE[c.status]}>{t(STATUS_LABEL[c.status])}</Badge>
                    {NEEDS_ATTENTION.includes(c.status) && (
                      <span className="ms-auto text-[11px] font-medium text-warning">
                        {t('needsAttention')}
                      </span>
                    )}
                  </span>
                  <span className="text-xs text-muted">
                    {new Date(c.updated).toLocaleString(lang === 'ar' ? 'ar' : 'en-GB')}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card>
        {!current ? (
          <p className="text-sm text-muted">{t('selectConversation')}</p>
        ) : (
          <ConversationPane
            conversation={current}
            messages={messages}
            leads={leads}
            canTakeOver={canTakeOver}
            onSetStatus={setStatus}
            workspaceId={workspaceId}
          />
        )}
      </Card>
    </div>
  );
}

function ConversationPane({
  conversation,
  messages,
  leads,
  canTakeOver,
  onSetStatus,
  workspaceId,
}: {
  conversation: Conversation;
  messages: ChatMessage[];
  leads: Lead[];
  canTakeOver: boolean;
  onSetStatus: (s: ConversationStatus) => void;
  workspaceId: string;
}) {
  const { t } = useLanguage();
  const [reply, setReply] = useState('');
  const [sending, setSending] = useState(false);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages]);

  const withHuman = conversation.status === 'human';

  async function send(e: FormEvent) {
    e.preventDefault();
    if (!reply.trim() || sending) return;
    setSending(true);
    try {
      // Role is `human`, and the collection rule permits only that role — an
      // agent must not be able to forge a visitor message or an unmetered
      // assistant one.
      await pb.collection('messages').create({
        workspace: workspaceId,
        conversation: conversation.id,
        role: 'human',
        text: reply.trim(),
      });
      setReply('');
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge tone={STATUS_TONE[conversation.status]}>
          {t(STATUS_LABEL[conversation.status])}
        </Badge>
        <div className="ms-auto flex flex-wrap gap-2">
          {canTakeOver && !withHuman && (
            <Button size="sm" variant="primary" onClick={() => onSetStatus('human')}>
              {t('takeOver')}
            </Button>
          )}
          {withHuman && (
            <Button size="sm" variant="secondary" onClick={() => onSetStatus('active')}>
              {t('returnToAuto')}
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={() => onSetStatus('closed')}>
            {t('markResolved')}
          </Button>
        </div>
      </div>

      {leads.length > 0 && (
        <div className="rounded-xl bg-surface-2 px-3 py-2 text-sm">
          <p className="text-xs text-muted">{t('leadContact')}</p>
          {leads.map((l) => (
            <p key={l.id} dir="auto">
              {l.name && <span className="font-medium">{l.name} · </span>}
              {/* A phone number or email is always LTR, even on an Arabic page. */}
              <span dir="ltr" className="font-mono text-xs">
                {l.contact}
              </span>
            </p>
          ))}
        </div>
      )}

      {withHuman && (
        // Said plainly, because it is the one thing an agent must be sure of
        // before they start typing.
        <p className="text-xs text-muted">{t('aiPaused')}</p>
      )}

      <div ref={listRef} className="flex max-h-80 flex-col gap-2 overflow-y-auto">
        {messages.map((m) => (
          <p
            key={m.id}
            // Per message: one conversation holds both languages.
            dir="auto"
            className={`max-w-[85%] whitespace-pre-wrap rounded-2xl px-3 py-2 text-sm ${
              m.role === 'user'
                ? 'self-start bg-surface-2 text-foreground'
                : m.role === 'human'
                  ? 'self-end bg-iris/12 text-foreground'
                  : 'self-end bg-surface-3 text-foreground'
            }`}
          >
            {m.text}
            {m.outcome === 'refused' && (
              <span className="mt-1 block text-[11px] text-muted">refused</span>
            )}
          </p>
        ))}
      </div>

      {canTakeOver ? (
        <form onSubmit={send} className="flex gap-2">
          <input
            value={reply}
            onChange={(e) => setReply(e.target.value)}
            placeholder={t('replyPlaceholder')}
            dir="auto"
            disabled={!withHuman || sending}
            className="grow rounded-xl border border-border-soft bg-surface-2 px-3 py-2 text-sm outline-none disabled:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2"
          />
          <Button type="submit" size="sm" variant="primary" disabled={!withHuman || sending}>
            {t('sendReply')}
          </Button>
        </form>
      ) : (
        <p className="text-xs text-muted">{t('takeoverNotOnPlan')}</p>
      )}
    </div>
  );
}
