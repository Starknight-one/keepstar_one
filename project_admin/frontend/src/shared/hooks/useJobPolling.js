import { useEffect, useRef, useState } from 'react'
import { api } from '../api/apiClient.js'

// useJobPolling follows an async import job until it reaches a terminal
// state. Used by the JSON import page and the CSV/Shopify onboarding flows —
// keeping the pattern in one place avoids three near-identical polling
// loops drifting apart.
//
// Options:
//   interval — ms between polls (default 2000)
//   endpoint — API path prefix the jobId gets appended to (default '/catalog/import/')
//   onComplete — fires once when status === completed
//
// Returns { job, isPolling, error }.
export default function useJobPolling(jobId, {
  interval = 2000,
  endpoint = '/catalog/import/',
  onComplete,
} = {}) {
  const [job, setJob] = useState(null)
  const [isPolling, setIsPolling] = useState(false)
  const [error, setError] = useState(null)
  const timerRef = useRef(null)
  const completeRef = useRef(onComplete)

  useEffect(() => { completeRef.current = onComplete }, [onComplete])

  useEffect(() => {
    if (!jobId) {
      setJob(null)
      setIsPolling(false)
      return
    }
    let cancelled = false
    setIsPolling(true)
    setError(null)

    const tick = async () => {
      if (cancelled) return
      try {
        const data = await api.get(`${endpoint}${jobId}`)
        if (cancelled) return
        setJob(data)
        if (data?.status === 'completed' || data?.status === 'failed') {
          setIsPolling(false)
          clearInterval(timerRef.current)
          timerRef.current = null
          if (data.status === 'completed' && completeRef.current) {
            completeRef.current(data)
          }
        }
      } catch (err) {
        if (cancelled) return
        setError(err)
      }
    }

    tick()
    timerRef.current = setInterval(tick, interval)
    return () => {
      cancelled = true
      if (timerRef.current) clearInterval(timerRef.current)
      timerRef.current = null
    }
  }, [jobId, interval, endpoint])

  return { job, isPolling, error }
}
