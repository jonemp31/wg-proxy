import { useState } from 'react'

export default function ProxyInfo({ device }) {
  const [showPass, setShowPass] = useState(false)
  const proxyUrl = device.proxy_url || `socks5://${device.proxy_user}:****@192.168.100.152:${device.proxy_port}`

  const copyProxy = () => {
    const fullUrl = `socks5://${device.proxy_user}:${device.proxy_pass || '****'}@192.168.100.152:${device.proxy_port}`
    navigator.clipboard.writeText(fullUrl)
  }

  return (
    <div className="flex items-center gap-2 text-xs">
      <code className="bg-gray-900 px-2 py-1 rounded text-gray-300 flex-1 truncate">
        {showPass ? proxyUrl.replace('****', device.proxy_pass || '****') : proxyUrl}
      </code>
      <button
        onClick={() => setShowPass(!showPass)}
        className="text-gray-500 hover:text-gray-300"
        title={showPass ? 'Ocultar senha' : 'Mostrar senha'}
      >
        {showPass ? '🙈' : '👁'}
      </button>
      <button
        onClick={copyProxy}
        className="bg-gray-700 hover:bg-gray-600 px-2 py-1 rounded"
      >
        Copiar
      </button>
    </div>
  )
}
