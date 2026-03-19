import { useState } from 'react'

export default function AddDeviceModal({ onAdd, onClose }) {
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState(null)
  const [error, setError] = useState(null)

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!name.trim()) return

    setLoading(true)
    setError(null)
    try {
      const data = await onAdd(name.trim())
      setResult(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text)
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50">
      <div className="bg-gray-800 rounded-lg p-6 w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <h2 className="text-lg font-bold mb-4">Adicionar Novo Dispositivo</h2>

        {!result ? (
          <form onSubmit={handleSubmit}>
            <label className="block text-sm text-gray-400 mb-1">Nome do dispositivo</label>
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="Ex: Motorola Cel1"
              className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-white mb-4"
              autoFocus
              disabled={loading}
            />

            {error && (
              <div className="bg-red-900/50 border border-red-700 text-red-300 px-3 py-2 rounded text-sm mb-4">
                {error}
              </div>
            )}

            <div className="flex gap-3">
              <button
                type="submit"
                disabled={loading || !name.trim()}
                className="bg-blue-600 hover:bg-blue-700 disabled:opacity-50 px-4 py-2 rounded text-sm font-medium flex-1"
              >
                {loading ? 'Gerando...' : 'Gerar Configuração'}
              </button>
              <button
                type="button"
                onClick={onClose}
                className="bg-gray-700 hover:bg-gray-600 px-4 py-2 rounded text-sm"
              >
                Cancelar
              </button>
            </div>
          </form>
        ) : (
          <div>
            <div className="text-center mb-4">
              <img
                src={`/api/devices/${result.device?.id || result.device_id}/qrcode`}
                alt="QR Code"
                className="mx-auto w-64 h-64 bg-white rounded p-2"
              />
              <p className="text-sm text-gray-400 mt-2">
                Escaneie com o app WireGuard no celular
              </p>
            </div>

            <div className="space-y-3">
              <div>
                <label className="block text-xs text-gray-400 mb-1">Proxy URL</label>
                <div className="flex gap-2">
                  <code className="flex-1 bg-gray-900 px-3 py-2 rounded text-xs text-green-400 break-all">
                    {result.proxy_url}
                  </code>
                  <button
                    onClick={() => copyToClipboard(result.proxy_url)}
                    className="bg-gray-700 hover:bg-gray-600 px-3 py-2 rounded text-xs"
                  >
                    Copiar
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-xs text-gray-400 mb-1">Config WireGuard</label>
                <div className="flex gap-2">
                  <code className="flex-1 bg-gray-900 px-3 py-2 rounded text-xs text-yellow-400 max-h-32 overflow-y-auto whitespace-pre">
                    {result.client_config}
                  </code>
                  <button
                    onClick={() => copyToClipboard(result.client_config)}
                    className="bg-gray-700 hover:bg-gray-600 px-3 py-2 rounded text-xs self-start"
                  >
                    Copiar
                  </button>
                </div>
              </div>
            </div>

            <button
              onClick={onClose}
              className="mt-4 w-full bg-gray-700 hover:bg-gray-600 px-4 py-2 rounded text-sm"
            >
              Fechar
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
