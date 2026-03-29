import { useState, useEffect } from 'react'
import { getWebhook, setWebhook, deleteWebhook, testWebhook } from '../services/api'

export default function WebhookModal({ onClose }) {
  const [url, setUrl] = useState('')
  const [saved, setSaved] = useState('')
  const [loading, setLoading] = useState(true)
  const [testing, setTesting] = useState(false)
  const [message, setMessage] = useState(null)

  useEffect(() => {
    getWebhook()
      .then(data => {
        setUrl(data.webhook_url || '')
        setSaved(data.webhook_url || '')
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const handleSave = async () => {
    if (!url.trim()) return
    try {
      await setWebhook(url.trim())
      setSaved(url.trim())
      setMessage({ type: 'success', text: 'Webhook salva!' })
    } catch (e) {
      setMessage({ type: 'error', text: e.message })
    }
  }

  const handleDelete = async () => {
    try {
      await deleteWebhook()
      setUrl('')
      setSaved('')
      setMessage({ type: 'success', text: 'Webhook removida' })
    } catch (e) {
      setMessage({ type: 'error', text: e.message })
    }
  }

  const handleTest = async () => {
    setTesting(true)
    setMessage(null)
    try {
      const result = await testWebhook()
      if (result.status === 'sent') {
        setMessage({ type: 'success', text: `Enviado! HTTP ${result.status_code}` })
      } else {
        setMessage({ type: 'error', text: result.error || 'Falha ao enviar' })
      }
    } catch (e) {
      setMessage({ type: 'error', text: e.message })
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-gray-800 rounded-lg p-6 w-full max-w-lg mx-4" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-bold mb-4">Configurar Webhook</h2>
        <p className="text-sm text-gray-400 mb-4">
          Receba notificações quando um proxy mudar de status (online, degraded, offline).
        </p>

        {loading ? (
          <div className="text-gray-400 text-center py-4">Carregando...</div>
        ) : (
          <>
            <label className="block text-sm text-gray-300 mb-1">URL da Webhook</label>
            <input
              type="url"
              value={url}
              onChange={e => setUrl(e.target.value)}
              placeholder="https://example.com/webhook"
              className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-sm mb-4 focus:outline-none focus:border-blue-500"
            />

            {message && (
              <div className={`text-sm px-3 py-2 rounded mb-4 ${
                message.type === 'success' 
                  ? 'bg-green-900/50 text-green-300 border border-green-700' 
                  : 'bg-red-900/50 text-red-300 border border-red-700'
              }`}>
                {message.text}
              </div>
            )}

            <div className="flex gap-2">
              <button
                onClick={handleSave}
                disabled={!url.trim() || url.trim() === saved}
                className="bg-blue-600 hover:bg-blue-700 disabled:opacity-40 disabled:cursor-not-allowed px-4 py-2 rounded text-sm font-medium"
              >
                Salvar
              </button>
              <button
                onClick={handleTest}
                disabled={!saved || testing}
                className="bg-gray-600 hover:bg-gray-500 disabled:opacity-40 disabled:cursor-not-allowed px-4 py-2 rounded text-sm font-medium"
              >
                {testing ? 'Enviando...' : 'Testar'}
              </button>
              <button
                onClick={handleDelete}
                disabled={!saved}
                className="bg-red-700 hover:bg-red-600 disabled:opacity-40 disabled:cursor-not-allowed px-4 py-2 rounded text-sm font-medium"
              >
                Remover
              </button>
              <button
                onClick={onClose}
                className="ml-auto bg-gray-700 hover:bg-gray-600 px-4 py-2 rounded text-sm font-medium"
              >
                Fechar
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
