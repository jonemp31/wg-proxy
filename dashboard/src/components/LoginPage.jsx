import { useState } from 'react'
import { login } from '../services/api'

export default function LoginPage({ onLogin }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(false)
  const [blocked, setBlocked] = useState(null)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError(null)
    setLoading(true)

    try {
      const data = await login(username, password)
      onLogin(data)
    } catch (err) {
      if (err.status === 429) {
        const seconds = err.retryAfter || 600
        setBlocked(seconds)
        setError(`Muitas tentativas. Aguarde ${Math.ceil(seconds / 60)} minutos.`)
        const timer = setTimeout(() => setBlocked(null), seconds * 1000)
        return () => clearTimeout(timer)
      }
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center">
      <div className="bg-gray-800 rounded-lg p-8 w-full max-w-sm mx-4 shadow-xl">
        <h1 className="text-2xl font-bold text-white text-center mb-2">WG Proxy Manager</h1>
        <p className="text-gray-400 text-sm text-center mb-6">Faça login para continuar</p>

        <form onSubmit={handleSubmit}>
          <div className="mb-4">
            <label className="block text-sm text-gray-300 mb-1">Usuário</label>
            <input
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-white text-sm focus:outline-none focus:border-blue-500"
              autoFocus
              required
            />
          </div>

          <div className="mb-6">
            <label className="block text-sm text-gray-300 mb-1">Senha</label>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-white text-sm focus:outline-none focus:border-blue-500"
              required
            />
          </div>

          {error && (
            <div className="bg-red-900/50 border border-red-700 text-red-300 px-3 py-2 rounded text-sm mb-4">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading || blocked}
            className="w-full bg-blue-600 hover:bg-blue-700 disabled:opacity-40 disabled:cursor-not-allowed text-white py-2 rounded font-medium text-sm"
          >
            {loading ? 'Entrando...' : 'Entrar'}
          </button>
        </form>
      </div>
    </div>
  )
}
