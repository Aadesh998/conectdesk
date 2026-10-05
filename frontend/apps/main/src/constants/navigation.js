export const reportsNavItems = [
  {
    titleKey: 'globals.terms.overview',
    href: '/reports/overview',
    permission: 'reports:manage',
    icon: 'BarChart3'
  },
  {
    titleKey: 'reports.calls',
    href: '/reports/calls',
    permission: 'reports:manage',
    icon: 'Phone'
  }
]

export const adminNavItems = [
  {
    titleKey: 'globals.terms.workspace',
    children: [
      {
        titleKey: 'globals.terms.general',
        href: '/admin/general',
        permission: 'general_settings:manage',
        icon: 'Settings'
      },
      {
        titleKey: 'admin.domains.title',
        href: '/admin/domains',
        permission: 'general_settings:manage',
        feature: 'tenancy',
        icon: 'Globe'
      },
      {
        titleKey: 'admin.usage.title',
        href: '/admin/usage',
        permission: 'general_settings:manage',
        icon: 'Gauge'
      },
      {
        titleKey: 'admin.admissions.title',
        href: '/admin/admissions',
        permission: 'general_settings:manage',
        icon: 'GraduationCap'
      },
      {
        titleKey: 'admin.privacy.title',
        href: '/admin/privacy',
        permission: 'general_settings:manage',
        icon: 'Lock'
      },
      {
        titleKey: 'admin.export.title',
        href: '/admin/export',
        permission: 'general_settings:manage',
        icon: 'Download'
      },
      {
        titleKey: 'globals.terms.businessHour',
        href: '/admin/business-hours',
        permission: 'business_hours:manage',
        isTitleKeyPlural: true,
        icon: 'Clock'
      },
      {
        titleKey: 'globals.terms.slaPolicy',
        href: '/admin/sla',
        permission: 'sla:manage',
        isTitleKeyPlural: true,
        icon: 'Timer'
      }
    ]
  },
  {
    titleKey: 'globals.terms.helpCenter',
    badge: 'BETA',
    children: [
      {
        titleKey: 'globals.terms.helpCenter',
        href: '/admin/help-center',
        permission: 'help_center:manage',
        icon: 'BookOpen'
      }
    ]
  },
  {
    titleKey: 'admin.ai.title',
    badge: 'BETA',
    children: [
      {
        titleKey: 'globals.terms.provider',
        href: '/admin/ai/providers',
        permission: 'ai:manage',
        isTitleKeyPlural: true,
        icon: 'Sparkles'
      },
      {
        titleKey: 'admin.ai.snippets',
        href: '/admin/ai/snippets',
        permission: 'ai:manage',
        isTitleKeyPlural: true,
        icon: 'NotebookText'
      },
      {
        titleKey: 'admin.ai.suggestions',
        href: '/admin/ai/suggestions',
        permission: 'ai:manage',
        isTitleKeyPlural: true,
        icon: 'Lightbulb'
      },
      {
        titleKey: 'admin.ai.tools',
        href: '/admin/ai/tools',
        permission: 'ai:manage',
        isTitleKeyPlural: true,
        icon: 'Wrench'
      },
      {
        titleKey: 'admin.ai.assistants',
        href: '/admin/ai/assistants',
        permission: 'ai:manage',
        isTitleKeyPlural: true,
        icon: 'Bot'
      }
    ]
  },
  {
    titleKey: 'globals.terms.channel',
    isTitleKeyPlural: true,
    children: [
      {
        titleKey: 'globals.terms.inbox',
        href: '/admin/inboxes',
        permission: 'inboxes:manage',
        isTitleKeyPlural: true,
        icon: 'Inbox'
      }
    ]
  },
  {
    titleKey: 'globals.terms.conversation',
    isTitleKeyPlural: true,
    children: [
      {
        titleKey: 'globals.terms.status',
        href: '/admin/conversations/statuses',
        permission: 'status:manage',
        isTitleKeyPlural: true,
        icon: 'CircleDot'
      },
      {
        titleKey: 'globals.terms.tag',
        href: '/admin/conversations/tags',
        permission: 'tags:manage',
        isTitleKeyPlural: true,
        icon: 'Tag'
      },
      {
        titleKey: 'globals.terms.customAttribute',
        href: '/admin/custom-attributes',
        permission: 'custom_attributes:manage',
        isTitleKeyPlural: true,
        icon: 'SlidersHorizontal'
      },
      {
        titleKey: 'globals.terms.sharedView',
        href: '/admin/conversations/shared-views',
        permission: 'shared_views:manage',
        isTitleKeyPlural: true,
        icon: 'Eye'
      }
    ]
  },
  {
    titleKey: 'globals.terms.productivity',
    children: [
      {
        titleKey: 'globals.terms.macro',
        href: '/admin/conversations/macros',
        permission: 'macros:manage',
        isTitleKeyPlural: true,
        icon: 'Zap'
      },
      {
        titleKey: 'globals.terms.automation',
        href: '/admin/automations',
        permission: 'automations:manage',
        isTitleKeyPlural: true,
        icon: 'Workflow'
      }
    ]
  },
  {
    titleKey: 'globals.terms.teammate',
    isTitleKeyPlural: true,
    children: [
      {
        titleKey: 'globals.terms.agent',
        href: '/admin/teams/agents',
        permission: 'users:manage',
        isTitleKeyPlural: true,
        icon: 'UserRound'
      },
      {
        titleKey: 'globals.terms.team',
        href: '/admin/teams/teams',
        permission: 'teams:manage',
        isTitleKeyPlural: true,
        icon: 'UsersRound'
      },
      {
        titleKey: 'globals.terms.role',
        href: '/admin/teams/roles',
        permission: 'roles:manage',
        isTitleKeyPlural: true,
        icon: 'Shield'
      },
      {
        titleKey: 'globals.terms.activityLog',
        href: '/admin/teams/activity-log',
        permission: 'activity_logs:manage',
        isTitleKeyPlural: true,
        icon: 'ScrollText'
      }
    ]
  },
  {
    titleKey: 'globals.terms.notification',
    isTitleKeyPlural: true,
    children: [
      {
        titleKey: 'globals.terms.email',
        href: '/admin/notification',
        permission: 'notification_settings:manage',
        icon: 'Mail'
      },
      {
        titleKey: 'globals.terms.template',
        href: '/admin/templates',
        permission: 'templates:manage',
        isTitleKeyPlural: true,
        icon: 'FileText'
      }
    ]
  },
  {
    titleKey: 'globals.terms.security',
    children: [
      {
        titleKey: 'globals.terms.sso',
        href: '/admin/sso',
        permission: 'oidc:manage',
        icon: 'KeyRound'
      }
    ]
  },
  {
    titleKey: 'globals.terms.integration',
    isTitleKeyPlural: true,
    children: [
      {
        titleKey: 'globals.terms.webhook',
        href: '/admin/webhooks',
        feature: 'integration:webhooks',
        permission: 'webhooks:manage',
        isTitleKeyPlural: true,
        icon: 'Webhook'
      },
      {
        titleKey: 'admin.chat.slack.title',
        href: '/admin/integrations/slack',
        feature: 'integration:slack',
        permission: 'webhooks:manage',
        icon: 'Slack'
      },
      {
        titleKey: 'admin.chat.teams.title',
        href: '/admin/integrations/teams',
        feature: 'integration:teams',
        permission: 'webhooks:manage',
        icon: 'UsersRound'
      },
      {
        titleKey: 'globals.terms.contextLink',
        href: '/admin/context-links',
        feature: 'integration:context_links',
        permission: 'context_links:manage',
        isTitleKeyPlural: true,
        icon: 'Link'
      }
    ]
  }
]

export const accountNavItems = [
  {
    titleKey: 'globals.terms.profile',
    href: '/account/profile',
    icon: 'CircleUser'
  },
  {
    titleKey: 'account.security',
    href: '/account/security',
    icon: 'ShieldCheck'
  }
]

export const contactNavItems = [
  {
    titleKey: 'globals.terms.contact',
    allLabelKey: 'contact.allContacts',
    href: '/contacts',
    isTitleKeyPlural: true,
    icon: 'Contact'
  }
]
