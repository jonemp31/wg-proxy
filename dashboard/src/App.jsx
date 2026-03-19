import { useState } from 'react'
import DeviceList from './components/DeviceList'
import AddDeviceModal from './components/AddDeviceModal'
import { useDevices } from './hooks/useDevices'

export default function App() {
  const { devices, loading, error, connected, addDevice, removeDevice, onlineCount } = useDevices()
  const [showModal, setShowModal] = useState(false)

  return (
    <div className="min-h-screen bg-gray-900 text-white">
      <header className="border-b border-gray-700 px-6 py-4 flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h1 className="text-xl font-bold">WG Proxy Manager</h1>
          <span className={`text-xs px-2 py-1 rounded ${connected ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}`}>
            {connected ? 'Live' : 'Desconectado'}
          </span>
        </div>
        <button
          onClick={() => setShowModal(true)}
          className="bg-blue-600 hover:bg-blue-700 px-4 py-2 rounded text-sm font-medium"
        >
          + Adicionar Device
        </button>
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
    </div>
  )
}
