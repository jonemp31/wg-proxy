import { useEffect, useRef, useState, useCallback } from 'react'

export function useWebSocket(url) {
  const [lastEvent, setLastEvent] = useState(null)
  const [connected, setConnected] = useState(false)
  const wsRef = useRef(null)
  const retriesRef = useRef(0)

  const connect = useCallback(() => {
    const wsUrl = url || `ws://${window.location.host}/ws`
    const ws = new WebSocket(wsUrl)
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
      retriesRef.current = 0
    }

    ws.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data)
        setLastEvent(event)
      } catch {}
    }

    ws.onclose = () => {
      setConnected(false)
      const delay = Math.min(1000 * Math.pow(2, retriesRef.current), 30000)
      retriesRef.current++
      setTimeout(connect, delay)
    }

    ws.onerror = () => ws.close()
  }, [url])

  useEffect(() => {
    connect()
    return () => {
      if (wsRef.current) wsRef.current.close()
    }
  }, [connect])

  return { lastEvent, connected }
}
