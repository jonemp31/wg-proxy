import { useState } from 'react'
import StatusBadge from './StatusBadge'
import ProxyInfo from './ProxyInfo'

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatRate(bytesPerSec) {
  if (!bytesPerSec || bytesPerSec === 0) return '0 B/s'
  return formatBytes(bytesPerSec) + '/s'
}

export default function DeviceCard({ device, onRemove }) {
  const [confirming, setConfirming] = useState(false)
  const healthStatus = device.health_status || (device.online ? 'online' : 'offline')
  const online = healthStatus !== 'offline'

  const borderColor = healthStatus === 'online' ? 'border-green-800' 
    : healthStatus === 'degraded' ? 'border-yellow-800' 
    : 'border-gray-700'

  const handleRemove = async () => {
    if (!confirming) {
      setConfirming(true)
      setTimeout(() => setConfirming(false), 3000)
      return
    }
    await onRemove(device.id)
  }

  return (
    <div className={`bg-gray-800 border rounded-lg p-4 ${borderColor}`}>
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-3">
          <StatusBadge online={online} healthStatus={healthStatus} />
          <div>
            <h3 className="font-semibold">{device.name}</h3>
            <p className="text-xs text-gray-400">
              WG: {device.wg_ip} | Porta: {device.proxy_port}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2 text-xs">
          {device.real_ip && (
            <span className="text-gray-400">
              IP: {device.real_ip}{device.isp ? ` | ${device.isp}` : ''}
            </span>
          )}
          <button
            onClick={handleRemove}
            className={`px-3 py-1 rounded text-xs ${confirming ? 'bg-red-600 text-white' : 'bg-gray-700 text-gray-300 hover:bg-red-900'}`}
          >
            {confirming ? 'Confirmar?' : 'Remover'}
          </button>
        </div>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
        <div className="bg-gray-900 rounded p-2">
          <div className="text-gray-500 text-xs mb-1">Velocidade</div>
          <div className="flex items-center gap-1 text-green-400">
            <span>↑</span>
            <span>{formatRate(device.tx_rate)}</span>
          </div>
          <div className="flex items-center gap-1 text-blue-400">
            <span>↓</span>
            <span>{formatRate(device.rx_rate)}</span>
          </div>
        </div>
        <div className="bg-gray-900 rounded p-2">
          <div className="text-gray-500 text-xs mb-1">Total Tráfego</div>
          <div className="flex items-center gap-1 text-green-400">
            <span>↑</span>
            <span>{formatBytes(device.tx_bytes)}</span>
          </div>
          <div className="flex items-center gap-1 text-blue-400">
            <span>↓</span>
            <span>{formatBytes(device.rx_bytes)}</span>
          </div>
        </div>
      </div>
      {(device.rx_bytes_24h > 0 || device.tx_bytes_24h > 0) && (
        <div className="mt-2 bg-gray-900 rounded p-2 text-sm">
          <div className="text-gray-500 text-xs mb-1">Últimas 24h</div>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-1 text-green-400">
              <span>↑</span>
              <span>{formatBytes(device.tx_bytes_24h)}</span>
            </div>
            <div className="flex items-center gap-1 text-blue-400">
              <span>↓</span>
              <span>{formatBytes(device.rx_bytes_24h)}</span>
            </div>
            <div className="text-gray-400 text-xs ml-auto">
              Total: {formatBytes((device.rx_bytes_24h || 0) + (device.tx_bytes_24h || 0))}
            </div>
          </div>
        </div>
      )}

      <div className="mt-3">
        <ProxyInfo device={device} />
      </div>
    </div>
  )
}
