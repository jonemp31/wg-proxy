import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from 'recharts'

function formatBytes(bytes) {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

export default function TrafficChart({ metrics }) {
  if (!metrics || metrics.length === 0) {
    return <div className="text-gray-500 text-sm text-center py-8">Sem dados de tráfego</div>
  }

  const data = metrics.map(m => ({
    time: new Date(m.recorded_at).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }),
    rx: m.rx_bytes,
    tx: m.tx_bytes,
  })).reverse()

  return (
    <div className="h-64">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data}>
          <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
          <XAxis dataKey="time" stroke="#9CA3AF" fontSize={11} />
          <YAxis stroke="#9CA3AF" fontSize={11} tickFormatter={formatBytes} />
          <Tooltip
            contentStyle={{ backgroundColor: '#1F2937', border: '1px solid #374151', borderRadius: 8 }}
            labelStyle={{ color: '#9CA3AF' }}
            formatter={(v) => formatBytes(v)}
          />
          <Line type="monotone" dataKey="rx" stroke="#60A5FA" strokeWidth={2} dot={false} name="Download" />
          <Line type="monotone" dataKey="tx" stroke="#34D399" strokeWidth={2} dot={false} name="Upload" />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
