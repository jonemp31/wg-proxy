import { useState, useEffect } from 'react'
import DeviceList from './components/DeviceList'
import AddDeviceModal from './components/AddDeviceModal'
import WebhookModal from './components/WebhookModal'
import ChangePasswordModal from './components/ChangePasswordModal'
import LoginPage from './components/LoginPage'
import { useDevices } from './hooks/useDevices'
import { hasToken, clearToken } from './services/api'

export default function App() {
  const [authenticated, setAuthenticated] = useState(hasToken())
  const { devices, loading, error, connected, addDevice, removeDevice, onlineCount } = useDevices()
  const [showModal, setShowModal] = useState(false)
  const [showWebhook, setShowWebhook] = useState(false)
  const [showPassword, setShowPassword] = useState(false)

  useEffect(() => {
    const handleLogout = () => setAuthenticated(false)
    window.addEventListener('auth:logout', handleLogout)
    return () => window.removeEventListener('auth:logout', handleLogout)
  }, [])

  const handleLogin = () => {
    setAuthenticated(true)
  }

  const handleLogout = () => {
    clearToken()
    setAuthenticated(false)
  }

  if (!authenticated) {
    return <LoginPage onLogin={handleLogin} />
  }

  return (
    <div className="min-h-screen bg-gray-900 text-white">
      <header className="border-b border-gray-700 px-6 py-4 flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h1 className="text-xl font-bold">WG Proxy Manager</h1>
          <span className={`text-xs px-2 py-1 rounded ${connected ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}`}>
            {connected ? 'Live' : 'Desconectado'}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setShowPassword(true)}
            className="bg-gray-700 hover:bg-gray-600 px-3 py-2 rounded text-sm"
            title="Alterar senha"
          >
            &#9881;
          </button>
          <button
            onClick={() => setShowWebhook(true)}
            className="bg-gray-700 hover:bg-gray-600 px-4 py-2 rounded text-sm font-medium"
          >
            Webhook
          </button>
          <button
            onClick={() => setShowModal(true)}
            className="bg-blue-600 hover:bg-blue-700 px-4 py-2 rounded text-sm font-medium"
          >
            + Adicionar Device
          </button>
          <button
            onClick={handleLogout}
            className="bg-red-800 hover:bg-red-700 px-3 py-2 rounded text-sm font-medium"
          >
            Sair
          </button>
        </div>
      </header>

      <main className="p-6">
        {error && (
          <div className="bg-red-900/50 border border-red-700 text-red-300 px-4 py-3 rounded mb-4">
            {error}
          </div>
        )}

        {loading ? (
          <div className="text-gray-400 text-center py-12">Carregando...</div>
        ) : (
          <>
            <div className="mb-4 text-sm text-gray-400">
              {devices.length} device{devices.length !== 1 ? 's' : ''} | {onlineCount} online | {devices.length - onlineCount} offline
            </div>
            <DeviceList devices={devices} onRemove={removeDevice} />
          </>
        )}
      </main>

      {showModal && (
        <AddDeviceModal
          onAdd={addDevice}
          onClose={() => setShowModal(false)}
        />
      )}

      {showWebhook && (
        <WebhookModal onClose={() => setShowWebhook(false)} />
      )}

      {showPassword && (
        <ChangePasswordModal onClose={() => setShowPassword(false)} />
      )}
    </div>
  )
}
