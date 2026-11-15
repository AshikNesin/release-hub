import {
  FileBadge,
  KeyRound,
  Palette,
  SlidersHorizontal,
  Store,
  TriangleAlert,
  Users,
} from "lucide-react";
import { Alert, AlertDescription } from "@groot/ui/alert";
import { monoClass } from "@groot/ui/typography";
import { ThemeSettings } from "@groot/shell/components/ThemeSettings";
import { Settings, type SettingsSection } from "@groot/shell/pages/settings/Settings";
import { useSettings } from "@/lib/releaseHub";
import { PlayAccountsSettings } from "./PlayAccountsSettings";
import { SettingsForm } from "./SettingsForm";
import { TokensSettings } from "./TokensSettings";

function GeneralSettings() {
  return (
    <>
      <SettingsForm
        title="Default package name"
        description="Prefills the package name when adding a platform: prefix + slug."
        fields={[
          {
            key: "bundle_prefix",
            label: "Bundle prefix",
            read: (s) => s.bundlePrefix,
            placeholder: "com.example",
            mono: true,
            hint: (
              <>
                <code className={monoClass}>com.example</code> +{" "}
                <code className={monoClass}>my-app</code> →{" "}
                <code className={monoClass}>com.example.my_app</code>
              </>
            ),
          },
        ]}
      />
      <SettingsForm
        title="Release retention"
        description="Keeps storage bounded: after each direct upload, older direct releases beyond this count are deleted with their artifacts. Apps can override it; Play channels are never pruned."
        fields={[
          {
            key: "prune_keep",
            label: "Keep newest direct releases per app",
            read: (s) => s.pruneKeep,
            placeholder: "Keep everything",
            inputMode: "numeric",
            hint: "Blank or 0 keeps everything.",
          },
        ]}
      />
    </>
  );
}

function TestersSettings() {
  return (
    <>
      <SettingsForm
        title="Beta tester groups"
        description="Google Groups pushed to each Play-enabled app's alpha (closed testing) track when you invite testers or publish to alpha."
        fields={[
          {
            key: "play_testers",
            label: "Google Group addresses (comma or newline separated)",
            read: (s) => s.playTesters,
            placeholder: "beta-testers@googlegroups.com",
            multiline: true,
            mono: true,
          },
        ]}
      >
        <Alert>
          <TriangleAlert className="text-warning" />
          <AlertDescription>
            Inviting replaces the track's tester list with exactly these groups. Testers added
            directly in Play Console are removed; manage testers here or in Console, not both.
          </AlertDescription>
        </Alert>
      </SettingsForm>
      <SettingsForm
        title="Individual tester emails"
        description="Google's API cannot push personal addresses. The release hub keeps this list as the source of truth: it's on every Play-enabled app's Distribution tab, ready to paste into Play Console's email lists, and served by GET /api/apps/{slug}/{platform}/testers."
        fields={[
          {
            key: "play_tester_emails",
            label: "Emails (comma or newline separated)",
            read: (s) => s.playTesterEmails,
            placeholder: "alice@gmail.com, bob@gmail.com",
            multiline: true,
            mono: true,
          },
        ]}
      />
    </>
  );
}

function CertificateSettings() {
  const { data } = useSettings();
  const fields = (data?.certFields ?? []).map((f) => ({
    key: f.field as "org",
    label: f.label,
    placeholder: f.placeholder,
    hint: f.hint,
    read: (s: NonNullable<typeof data>) =>
      s.certFields.find((c) => c.field === f.field)?.value ?? "",
  }));
  if (!data) return <SettingsForm title="Certificate subject" fields={[]} />;
  return (
    <SettingsForm
      title="Certificate subject"
      description="The distinguished name baked into signing keys generated from now on. Existing keystores keep their certificate."
      fields={fields}
      columns={2}
    />
  );
}

const SECTIONS: SettingsSection[] = [
  {
    id: "general",
    label: "General",
    group: "Release hub",
    icon: SlidersHorizontal,
    description: "Defaults applied across every app.",
    keywords: ["bundle", "prefix", "package", "retention", "prune", "storage"],
    items: [
      { label: "Bundle prefix" },
      { label: "Release retention", keywords: ["prune", "keep"] },
    ],
    element: <GeneralSettings />,
  },
  {
    id: "testers",
    label: "Testers",
    group: "Release hub",
    icon: Users,
    description: "Who gets Play testing builds.",
    keywords: ["beta", "alpha", "google groups", "email", "closed testing"],
    items: [{ label: "Beta tester groups" }, { label: "Individual tester emails" }],
    element: <TestersSettings />,
  },
  {
    id: "signing",
    label: "Signing",
    group: "Release hub",
    icon: FileBadge,
    description: "Certificate details for generated keystores.",
    keywords: ["keystore", "certificate", "organization", "country", "dn"],
    items: [{ label: "Certificate subject" }],
    element: <CertificateSettings />,
  },
  {
    id: "tokens",
    label: "API tokens",
    group: "Access",
    icon: KeyRound,
    description: "Bearer tokens for CI and deploy scripts.",
    keywords: ["token", "bearer", "ci", "api", "revoke"],
    items: [{ label: "Create a token" }, { label: "Revoke a token" }],
    element: <TokensSettings />,
  },
  {
    id: "google-play",
    label: "Google Play",
    group: "Access",
    icon: Store,
    description: "Shared service accounts that publish to Play.",
    keywords: ["service account", "json", "key", "play console", "publishing"],
    items: [{ label: "Add a service account" }, { label: "Remove a service account" }],
    element: <PlayAccountsSettings />,
  },
  {
    id: "appearance",
    label: "Appearance",
    group: "Personal",
    icon: Palette,
    description: "How release-hub looks on this device.",
    keywords: ["theme", "dark", "light", "system"],
    items: [{ label: "Theme", description: "Light, dark or system" }],
    element: <ThemeSettings />,
  },
];

export function ReleaseHubSettingsPage() {
  return <Settings sections={SECTIONS} />;
}
