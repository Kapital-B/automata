interface IconProps {
  className?: string;
}

export function GoogleIcon({ className }: IconProps) {
  return (
    <svg className={className} viewBox="0 0 24 24" aria-hidden>
      <path
        fill="#EA4335"
        d="M12 10.2v3.9h5.5c-.24 1.4-1.7 4.1-5.5 4.1-3.3 0-6-2.7-6-6.1s2.7-6.1 6-6.1c1.9 0 3.1.8 3.8 1.5l2.6-2.5C16.9 3.5 14.7 2.5 12 2.5 6.8 2.5 2.6 6.7 2.6 12s4.2 9.5 9.4 9.5c5.4 0 9-3.8 9-9.2 0-.6-.06-1.1-.16-1.6H12z"
      />
      <path
        fill="#34A853"
        d="M3.6 7.5l3.2 2.4C7.7 7.7 9.7 6.4 12 6.4c1.9 0 3.1.8 3.8 1.5l2.6-2.5C16.9 3.5 14.7 2.5 12 2.5 8.3 2.5 5 4.6 3.6 7.5z"
        opacity="0"
      />
      <path
        fill="#4285F4"
        d="M21.4 12.3c0-.6-.06-1.1-.16-1.6H12v3.9h5.5c-.24 1.4-1 2.6-2.1 3.4l3.3 2.6c1.9-1.8 3-4.4 3-8.3z"
      />
      <path
        fill="#FBBC05"
        d="M5.9 14.3c-.2-.6-.3-1.3-.3-2s.1-1.4.3-2L2.7 7.9C2 9.2 1.6 10.6 1.6 12s.4 2.8 1.1 4.1l3.2-2.4z"
      />
      <path
        fill="#34A853"
        d="M12 21.5c2.7 0 4.9-.9 6.6-2.4l-3.3-2.6c-.9.6-2 1-3.3 1-2.6 0-4.7-1.7-5.5-4.1l-3.3 2.5c1.5 3 4.6 5.6 8.8 5.6z"
      />
    </svg>
  );
}

export function MicrosoftIcon({ className }: IconProps) {
  return (
    <svg className={className} viewBox="0 0 24 24" aria-hidden>
      <path fill="#F25022" d="M2 2h9.5v9.5H2z" />
      <path fill="#7FBA00" d="M12.5 2H22v9.5h-9.5z" />
      <path fill="#00A4EF" d="M2 12.5h9.5V22H2z" />
      <path fill="#FFB900" d="M12.5 12.5H22V22h-9.5z" />
    </svg>
  );
}
