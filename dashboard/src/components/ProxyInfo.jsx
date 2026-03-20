import { useState } from 'react'

export default function ProxyInfo({ device }) {
  const [showPass, setShowPass] = useState(false)
  const fullUrl = device.proxy_url || `socks5://${device.proxy_user}:****@192.168.100.152:${device.proxy_port}`
  const maskedUrl = fullUrl.replace(/:([^:@]+)@/, ':****@')

  const [copied, setCopied] = useState(false)

  const copyProxy = () => {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(fullUrl)
      } else {
        const ta = document.createElement('textarea')
        ta.value = fullUrl
        ta.style.position = 'fixed'
        ta.style.left = '-9999px'
        document.body.appendChild(ta)
        ta.select()
        document.execCommand('copy')
        document.body.removeChild(ta)
      }
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (e) {
      console.error('Copy failed', e)
    }
  }

  return (
    <div className="flex items-center gap-2 text-xs">
      <code className="bg-gray-900 px-2 py-1 rounded text-gray-300 flex-1 truncate">
        {showPass ? fullUrl : maskedUrl}
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
        className={`${copied ? 'bg-green-700' : 'bg-gray-700 hover:bg-gray-600'} px-2 py-1 rounded`}
      >
        {copied ? '✓' : 'Copiar'}
      </button>
    </div>
  )
}
