export default function Alert({ message }: { message: string }) {
  return (
    <p role="alert" className="rounded bg-red-950 px-3 py-2 text-sm text-red-200">
      {message}
    </p>
  )
}
