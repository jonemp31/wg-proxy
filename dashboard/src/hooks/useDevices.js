import { useState, useEffect, useCallback } from 'react'
import { getDevices, createDevice, deleteDevice } from '../services/api'
import { useWebSocket } from './useWebSocket'

export function useDevices() {
  const [devices, setDevices] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const { lastEvent, connected } = useWebSocket()

  const refresh = useCallback(async () => {
    try {
      setError(null)
      const data = await getDevices()
      setDevices(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  useEffect(() => {
    if (!lastEvent) return

    switch (lastEvent.type) {
      case 'device_created':
      case 'device_removed':
        refresh()
        break
      case 'device_online':
      case 'device_offline':
        refresh()
        break
      case 'metrics_update':
        if (Array.isArray(lastEvent.data)) {
          setDevices(prev => prev.map(dev => {
            const update = lastEvent.data.find(u => u.id === dev.id)
            if (update) {
              return {
                ...dev,
                status: update.status,
                online: update.status === 'online',
                health_status: update.health_status || update.status,
                rx_rate: update.rx_rate,
                tx_rate: update.tx_rate,
                rx_bytes: update.rx_bytes ?? dev.rx_bytes,
                tx_bytes: update.tx_bytes ?? dev.tx_bytes,
                real_ip: update.real_ip,
              }
            }
            return dev
          }))
        }
        break
    }
  }, [lastEvent, refresh])

  const addDevice = useCallback(async (name) => {
    const result = await createDevice(name)
    await refresh()
    return result
  }, [refresh])

  const removeDevice = useCallback(async (id) => {
    await deleteDevice(id)
    await refresh()
  }, [refresh])

  const onlineCount = devices.filter(d => {
    const hs = d.health_status || (d.online ? 'online' : 'offline')
    return hs !== 'offline'
  }).length

  return { devices, loading, error, connected, addDevice, removeDevice, refresh, onlineCount }
}
