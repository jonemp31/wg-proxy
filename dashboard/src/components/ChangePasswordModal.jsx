import { useState } from 'react'
import { changePassword } from '../services/api'

export default function ChangePasswordModal({ onClose }) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [message, setMessage] = useState(null)
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setMessage(null)

    if (newPassword !== confirmPassword) {
      setMessage({ type: 'error', text: 'As senhas não coincidem' })
      return
    }
    if (newPassword.length < 6) {
      setMessage({ type: 'error', text: 'Senha deve ter pelo menos 6 caracteres' })
      return
    }

    setLoading(true)
    try {
      await changePassword(currentPassword, newPassword)
      setMessage({ type: 'success', text: 'Senha alterada com sucesso!' })
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (err) {
      setMessage({ type: 'error', text: err.message })
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-gray-800 rounded-lg p-6 w-full max-w-md mx-4" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-bold mb-4">Alterar Senha</h2>

        <form onSubmit={handleSubmit}>
          <div className="mb-3">
            <label className="block text-sm text-gray-300 mb-1">Senha atual</label>
            <input
              type="password"
              value={currentPassword}
              onChange={e => setCurrentPassword(e.target.value)}
              className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-blue-500"
              required
            />
          </div>

          <div className="mb-3">
            <label className="block text-sm text-gray-300 mb-1">Nova senha</label>
            <input
              type="password"
              value={newPassword}
              onChange={e => setNewPassword(e.target.value)}
              className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-blue-500"
              required
            />
          </div>

          <div className="mb-4">
            <label className="block text-sm text-gray-300 mb-1">Confirmar nova senha</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={e => setConfirmPassword(e.target.value)}
              className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-blue-500"
              required
            />
          </div>

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
              type="submit"
              disabled={loading}
              className="bg-blue-600 hover:bg-blue-700 disabled:opacity-40 px-4 py-2 rounded text-sm font-medium"
            >
              {loading ? 'Salvando...' : 'Alterar Senha'}
            </button>
            <button
              type="button"
              onClick={onClose}
              className="ml-auto bg-gray-700 hover:bg-gray-600 px-4 py-2 rounded text-sm font-medium"
            >
              Fechar
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
