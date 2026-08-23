import { Check, Clipboard, Download, KeyRound, Loader2, Plus, ShieldCheck, Trash2 } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { type PasskeyEnrollmentResult, type PasskeyStatus, passkeys } from "@/lib/api"
import { createPasskey, getPasskeyAssertion } from "@/lib/webauthn"

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Passkey operation failed"
}

export function RecoveryCodes({ codes, onAcknowledged }: { codes: string[]; onAcknowledged: () => void }) {
  const [copied, setCopied] = useState(false)
  const text = codes.join("\n")

  const copyCodes = async () => {
    await navigator.clipboard.writeText(text)
    setCopied(true)
  }

  const downloadCodes = () => {
    const url = URL.createObjectURL(new Blob([`${text}\n`], { type: "text/plain;charset=utf-8" }))
    const link = document.createElement("a")
    link.href = url
    link.download = "licensehub-recovery-codes.txt"
    link.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Card className="border-amber-500/50">
      <CardHeader>
        <CardTitle className="text-base">Save your recovery codes now</CardTitle>
        <CardDescription>
          Each code works once. They are not stored in readable form and will never be shown again.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <fieldset className="grid gap-2 rounded-md bg-muted p-4 font-mono text-sm sm:grid-cols-2">
          <legend className="sr-only">Recovery codes</legend>
          {codes.map((code) => (
            <span key={code}>{code}</span>
          ))}
        </fieldset>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={copyCodes}>
            {copied ? <Check className="mr-2 h-4 w-4" /> : <Clipboard className="mr-2 h-4 w-4" />}
            {copied ? "Copied" : "Copy codes"}
          </Button>
          <Button type="button" variant="outline" onClick={downloadCodes}>
            <Download className="mr-2 h-4 w-4" /> Download
          </Button>
          <Button type="button" onClick={onAcknowledged}>
            I saved the codes
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

export function PasskeyEnrollment({ onComplete }: { onComplete: () => void }) {
  const [name, setName] = useState(() => `${navigator.platform || "Browser"} passkey`)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [result, setResult] = useState<PasskeyEnrollmentResult | null>(null)

  const enroll = async () => {
    setBusy(true)
    setError("")
    try {
      const begin = await passkeys.registerBegin()
      const credential = await createPasskey(begin.options)
      const enrolled = await passkeys.registerFinish(begin.ceremony_id, name, credential)
      setResult(enrolled)
      if (!enrolled.recovery_codes?.length) onComplete()
    } catch (operationError) {
      setError(errorMessage(operationError))
    } finally {
      setBusy(false)
    }
  }

  if (result?.recovery_codes?.length) {
    return <RecoveryCodes codes={result.recovery_codes} onAcknowledged={onComplete} />
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <KeyRound className="h-4 w-4" /> Enroll an admin passkey
        </CardTitle>
        <CardDescription>Use Windows Hello, Touch ID, or a hardware security key.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="passkey-name">Passkey name</Label>
          <Input id="passkey-name" value={name} maxLength={80} onChange={(event) => setName(event.target.value)} />
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button type="button" onClick={enroll} disabled={busy || !name.trim()}>
          {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Plus className="mr-2 h-4 w-4" />}
          Create passkey
        </Button>
      </CardContent>
    </Card>
  )
}

export function PasskeyAssertion({ onComplete }: { onComplete: () => void }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  const verify = async () => {
    setBusy(true)
    setError("")
    try {
      const begin = await passkeys.assertionBegin()
      const credential = await getPasskeyAssertion(begin.options)
      await passkeys.assertionFinish(begin.ceremony_id, credential)
      onComplete()
    } catch (operationError) {
      setError(errorMessage(operationError))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck className="h-4 w-4" /> Verify your admin passkey
        </CardTitle>
        <CardDescription>
          Admin actions stay locked until this phishing-resistant verification completes.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button type="button" onClick={verify} disabled={busy}>
          {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Verify passkey
        </Button>
      </CardContent>
    </Card>
  )
}

export function PasskeyManager() {
  const [status, setStatus] = useState<PasskeyStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [adding, setAdding] = useState(false)
  const [deleting, setDeleting] = useState("")

  const load = useCallback(async () => {
    setError("")
    try {
      setStatus(await passkeys.status())
    } catch (loadError) {
      setError(errorMessage(loadError))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const remove = async (id: string) => {
    if (!window.confirm("Remove this passkey? The last passkey cannot be removed here.")) return
    setDeleting(id)
    setError("")
    try {
      await passkeys.remove(id)
      await load()
    } catch (removeError) {
      setError(errorMessage(removeError))
    } finally {
      setDeleting("")
    }
  }

  if (loading) return <div className="h-24 animate-pulse rounded-lg bg-muted" />
  if (adding)
    return (
      <PasskeyEnrollment
        onComplete={() => {
          setAdding(false)
          void load()
        }}
      />
    )

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck className="h-4 w-4" /> Admin passkeys
        </CardTitle>
        <CardDescription>Manage the phishing-resistant credentials allowed to approve admin actions.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="space-y-2">
          {(status?.passkeys || []).map((passkey) => (
            <div key={passkey.id} className="flex items-center justify-between rounded-md border p-3">
              <div>
                <p className="text-sm font-medium">{passkey.name}</p>
                <p className="text-xs text-muted-foreground">
                  Added {new Date(passkey.created_at).toLocaleString()}
                  {passkey.last_used_at ? ` · Last used ${new Date(passkey.last_used_at).toLocaleString()}` : ""}
                </p>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`Remove ${passkey.name}`}
                disabled={deleting === passkey.id}
                onClick={() => remove(passkey.id)}
              >
                {deleting === passkey.id ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Trash2 className="h-4 w-4 text-destructive" />
                )}
              </Button>
            </div>
          ))}
        </div>
        <Button type="button" variant="outline" onClick={() => setAdding(true)}>
          <Plus className="mr-2 h-4 w-4" /> Add passkey
        </Button>
      </CardContent>
    </Card>
  )
}
