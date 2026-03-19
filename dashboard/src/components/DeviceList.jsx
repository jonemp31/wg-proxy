import DeviceCard from './DeviceCard'

export default function DeviceList({ devices, onRemove }) {
  if (devices.length === 0) {
    return (
      <div className="text-center py-16 text-gray-500">
        <p className="text-lg">Nenhum device configurado</p>
        <p className="text-sm mt-2">Clique em "Adicionar Device" para começar</p>
      </div>
    )
  }

  return (
    <div className="grid gap-4">
      {devices.map(device => (
        <DeviceCard key={device.id} device={device} onRemove={onRemove} />
      ))}
    </div>
  )
}
