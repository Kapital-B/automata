import type { MailServerSettings } from "@/lib/auth";

export type MailPreset = {
  id: string;
  name: string;
  domains: string[];
  imap: MailServerSettings;
  smtp: MailServerSettings;
  /** Shown under the password field when the provider needs an app password. */
  passwordHint?: string;
};

// Well-known IMAP/SMTP settings, so most people never type a host name.
export const mailPresets: MailPreset[] = [
  {
    id: "gmail",
    name: "Gmail (app password)",
    domains: ["gmail.com", "googlemail.com"],
    imap: { host: "imap.gmail.com", port: 993, security: "tls" },
    smtp: { host: "smtp.gmail.com", port: 465, security: "tls" },
    passwordHint:
      "Use an app password from your Google Account security settings (needs 2-Step Verification), not your Google password.",
  },
  {
    id: "outlook",
    name: "Outlook.com / Microsoft 365",
    domains: ["outlook.com", "hotmail.com", "live.com"],
    imap: { host: "outlook.office365.com", port: 993, security: "tls" },
    smtp: { host: "smtp.office365.com", port: 587, security: "starttls" },
    passwordHint: "Microsoft accounts connect better with the Microsoft option, which needs no password.",
  },
  {
    id: "fastmail",
    name: "Fastmail",
    domains: ["fastmail.com", "fastmail.fm"],
    imap: { host: "imap.fastmail.com", port: 993, security: "tls" },
    smtp: { host: "smtp.fastmail.com", port: 465, security: "tls" },
    passwordHint: "Fastmail requires an app password for IMAP.",
  },
  {
    id: "icloud",
    name: "iCloud Mail",
    domains: ["icloud.com", "me.com", "mac.com"],
    imap: { host: "imap.mail.me.com", port: 993, security: "tls" },
    smtp: { host: "smtp.mail.me.com", port: 587, security: "starttls" },
    passwordHint: "Use an app-specific password from your Apple Account.",
  },
  {
    id: "yahoo",
    name: "Yahoo Mail",
    domains: ["yahoo.com", "ymail.com"],
    imap: { host: "imap.mail.yahoo.com", port: 993, security: "tls" },
    smtp: { host: "smtp.mail.yahoo.com", port: 465, security: "tls" },
    passwordHint: "Use an app password from your Yahoo account security settings.",
  },
  {
    id: "zoho",
    name: "Zoho Mail",
    domains: ["zoho.com", "zohomail.com"],
    imap: { host: "imap.zoho.com", port: 993, security: "tls" },
    smtp: { host: "smtp.zoho.com", port: 465, security: "tls" },
  },
];

export function presetForEmail(email: string): MailPreset | undefined {
  const domain = email.trim().toLowerCase().split("@")[1];
  if (!domain) return undefined;
  return mailPresets.find((p) => p.domains.includes(domain));
}
