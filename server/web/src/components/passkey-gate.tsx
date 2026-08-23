import type { ReactNode } from "react"
import { useCallback, useEffect, useState } from "react"
import { PasskeyAssertion, PasskeyEnrollment } from "@/components/passkey-security"
import { type PasskeyStatus, passkeys } from "@/lib/api"

export function AdminPasskeyGate({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<PasskeyStatus | null>(null)
  const [error, setError] = useState("")

  const load = useCallback(async () => {
    setError("")
    try {
      setStatus(await passkeys.status())
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : "Unable to load passkey status")
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  if (!status) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
        {error ? (
          <div className="space-y-3 text-center">
            <p className="text-sm text-destructive">{error}</p>
            <button type="button" className="text-sm underline" onClick={load}>
              Retry
            </button>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">Checking admin security…</p>
        )}
      </div>
    )
  }

  if (status.passkeys.length === 0) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
        <div className="w-full max-w-xl">
          <PasskeyEnrollment onComplete={load} />
        </div>
      </div>
    )
  }

  if (!status.step_up_verified) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
        <div className="w-full max-w-lg">
          <PasskeyAssertion onComplete={load} />
        </div>
      </div>
    )
  }

  return children
}
