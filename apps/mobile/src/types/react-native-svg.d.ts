// phosphor-react-native ships TypeScript sources and passes `className` to <Svg>, which
// react-native-svg accepts at runtime (NativeWind) but does not declare. Without this the
// icon library's own source fails our typecheck.
import 'react-native-svg';

declare module 'react-native-svg' {
  interface SvgProps {
    className?: string;
  }
}
